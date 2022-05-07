package store

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/certifi/gocertifi"
	"github.com/maxmcd/bramble/internal/logger"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/sandbox"
	"github.com/maxmcd/bramble/pkg/textreplace"
	"github.com/maxmcd/reptar"
	"github.com/mholt/archiver/v3"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (s *Store) NewBuilder(lockfileWriter types.LockfileWriter) *Builder {
	return &Builder{
		store:          s,
		lockfileWriter: lockfileWriter,
	}
}

type Builder struct {
	store          *Store
	lockfileWriter types.LockfileWriter
}

type BuildDerivationOptions struct {
	// ForceBuild will make sure we build even if the derivation already exists
	ForceBuild bool

	Shell   bool
	Verbose bool
}

func (b *Builder) BuildDerivation(ctx context.Context, drv Derivation, opts BuildDerivationOptions) (builtDrv Derivation, didBuild bool, err error) {
	select {
	case <-ctx.Done():
		return Derivation{}, false, context.Canceled
	default:
	}
	var span trace.Span
	ctx, span = tracer.Start(ctx, "store.BuildDerivation")
	defer span.End()
	span.SetAttributes(attribute.String("name", drv.Name))

	drv = formatDerivation(drv)

	outputs, drvExists, err := b.store.checkForBuiltDerivationOutputs(drv)
	drv.Outputs = outputs
	if err != nil {
		return drv, false, err
	}

	outputsExist := false
	if drvExists {
		outputsExist, err = b.store.outputFoldersExist(outputs)
		if err != nil {
			return drv, false, err
		}
	}

	filename := drv.Filename()
	span.SetAttributes(attribute.String("filename", filename))
	if drvExists && outputsExist && !opts.ForceBuild {
		return drv, false, nil
	}
	// logger.Print("Building derivation", filename)
	logger.Debugw(drv.PrettyJSON())
	if drv, err = b.buildDerivation(ctx, drv, opts); err != nil {
		return drv, false, errors.Wrap(err, "error building "+filename)
	}
	_, err = b.store.WriteDerivation(drv)
	// TODO: lock store on write
	return drv, true, err
}

func (b *Builder) buildDerivation(ctx context.Context, drv Derivation, opts BuildDerivationOptions) (Derivation, error) {
	var err error
	var span trace.Span
	ctx, span = tracer.Start(ctx, "store.buildDerivation")
	defer span.End()

	buildDir, err := b.store.storeLengthTempDir()
	if err != nil {
		return drv, err
	}
	if drv.Source.Path != "" {
		if err = fileutil.CopyDirectory(b.store.joinStorePath(drv.Source.Path), buildDir); err != nil {
			err = errors.Wrap(err, "error copying sources into build dir")
			return drv, err
		}
	}
	outputPaths := map[string]string{}
	for _, name := range drv.OutputNames {
		if outputPaths[name], err = b.store.storeLengthTempDir(); err != nil {
			return drv, err
		}
	}
	drvCopy, err := drv.copyWithOutputValuesReplaced()
	if err != nil {
		return drv, err
	}

	if opts.Shell && (drv.Builder == "basic_fetch_url" || drv.Builder == "fetch_git") {
		return drv, errors.New("can't spawn a shell with a builtin builder")
	}

	defer func() {
		// If we exit let's try and clean these paths up in case they still exist
		// TODO: could probably limit this to only when there's an error
		os.RemoveAll(buildDir)
		for _, outputPath := range outputPaths {
			_ = os.RemoveAll(outputPath)
		}
	}()

	switch drv.Builder {
	case "basic_fetch_url":
		err = b.fetchURLBuilder(ctx, drvCopy, outputPaths)
	default:
		err = b.regularBuilder(ctx, drvCopy, buildDir, outputPaths, opts)
	}
	if err != nil {
		return drv, err
	}

	var outputs map[string]Output

	outputs, err = b.store.hashAndMoveBuildOutputs(ctx, drv, outputPaths, buildDir)
	err = errors.Wrap(err, "hash and move build outputs") // noop if err is nil
	if err != nil {
		return drv, err
	}

	drv.Outputs, err = outputsToOutput(drv.OutputNames, outputs)

	switch {
	case drv.Builder == "basic_fetch_url":
		return drv, b.checkFetchDerivationHashes(drv, "basic_fetch_url "+drv.Env["url"])

	// These two are just a contract with an environment variable. Any
	// derivation could set these. I think that's ok from a security standpoint,
	// although could indeed populate the lockfile with junk. Hard to think of
	// another way to identify the fetcher derivations.

	// TODO: maybe better to have this be a single environment variable and put
	// the git logic into the derivation itself. Then other derivations could
	// use this for their own lockfile needs. Would need to be sure it can't be
	// abused to interact with our expected lockfile values.
	case drv.Env["confirm_fetch_url"] == "true":
		return drv, b.checkFetchDerivationHashes(drv, "fetch_url "+drv.Env["url"])
	case drv.Env["confirm_fetch_git"] == "true":
		url := "fetch_git " + drv.Env["url"]
		reference := drv.Env["reference"]
		if reference != "" {
			url += "@" + reference
		}
		return drv, b.checkFetchDerivationHashes(drv, url)
	}

	return drv, err
}

func (b *Builder) checkFetchDerivationHashes(drv Derivation, url string) error {
	// Check for a hash in the derivation
	hash := drv.Env["hash"]
	if hash == "" {
		// If we don't have that then check in the config map for an
		// existing value
		if existingHash, found := b.lockfileWriter.LookupEntry(url); found {
			hash = existingHash
		}
	}
	outputPath := drv.output("out").Path
	// If we have a hash to validate, ensure it's valid
	if hash != "" && outputPath != hash {
		return errors.Errorf(
			"Urlfetch for %q content doesn't match with the existing hash. "+
				"Hash %q was provided by the output was %q",
			url, hash, outputPath)
	}
	// If we never had a hash to validate, add it to lockfile
	if hash == "" {
		if err := b.lockfileWriter.AddEntry(url, outputPath); err != nil {
			return err
		}
	}
	return nil
}

func (b *Builder) fetchURLBuilder(ctx context.Context, drv Derivation, outputPaths map[string]string) (err error) {
	var span trace.Span
	ctx, span = tracer.Start(ctx, "store.fetchURLBuilder")
	defer span.End()
	if _, ok := outputPaths["out"]; len(outputPaths) > 1 || !ok {
		return errors.New("the fetch_url builder can only have the defalt output \"out\"")
	}
	url, ok := drv.Env["url"]
	if !ok {
		return errors.New("fetch_url requires the environment variable 'url' to be set")
	}
	// derivation can provide a hash, but usually this is just in the lockfile
	dir, path, err := b.downloadFile(ctx, url)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if strings.HasSuffix(drv.Env["url"], ".tar.gz") {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		r, err := gzip.NewReader(f)
		if err != nil {
			return errors.Wrap(err, "requires gzip-compressed body")
		}
		if err = reptar.Unarchive(r, outputPaths["out"]); err != nil {
			return err
		}
		return nil
	}
	// If it's not .tar.gz just put the file in the output
	return os.Rename(path, filepath.Join(outputPaths["out"], filepath.Base(url)))
}

// downloadFile downloads a file into a temp dir
func (b *Builder) downloadFile(ctx context.Context, url string) (dir, path string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// TODO: consider making this whole thing a derivation that is run with the
	// network. Cert mgmt should be bramble package tree thing not an in-code
	// thing.
	certPool, err := gocertifi.CACerts()
	transport.TLSClientConfig = &tls.Config{RootCAs: certPool}

	client := http.Client{
		Transport: transport,
	}
	resp, err := client.Do(req)
	if err != nil {
		err = errors.Wrap(err, fmt.Sprintf("error making request to download %q", url))
		return
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
	default:
		return "", "", errors.Errorf("Unexpected http status code %d when fetching url %q", resp.StatusCode, url)
	}
	defer resp.Body.Close()
	dir, err = os.MkdirTemp("", "")
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	f, err := os.Create(filepath.Join(dir, filepath.Base(url)))
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", "", errors.WithStack(err)
	}
	if err := f.Close(); err != nil {
		return "", "", errors.WithStack(err)
	}
	return dir, f.Name(), nil
}

func (b *Builder) regularBuilder(ctx context.Context, drv Derivation, buildDir string,
	outputPaths map[string]string, opts BuildDerivationOptions) (err error) {
	var span trace.Span
	ctx, span = tracer.Start(ctx, "store.regularBuilder")
	defer span.End()
	builderLocation := drv.Builder
	if _, err := os.Stat(builderLocation); err != nil {
		return errors.Wrap(err, "builder location doesn't exist")
	}
	env := drv.env()
	mounts := []string{
		b.store.StorePath + ":ro",
		buildDir,
	}
	for outputName, outputPath := range outputPaths {
		env = append(env, fmt.Sprintf("%s=%s", outputName, outputPath))
		mounts = append(mounts, outputPath)
	}
	var stdout io.Writer = os.Stdout
	var stderr io.Writer = os.Stderr
	var f *os.File
	var buf *bufio.Writer
	if !opts.Verbose {
		f, err = os.CreateTemp("", "")
		if err != nil {
			return err
		}
		buf = bufio.NewWriter(f)
		stdout, stderr = buf, buf
		// Clean up if we use the
		defer func() {
			if err != nil {
				_ = buf.Flush()
				return
			}
			_ = f.Close()
			err = os.Remove(f.Name())
		}()
	}
	sbx := sandbox.Sandbox{
		Args:    append([]string{builderLocation}, drv.Args...),
		Stdout:  stdout,
		Network: drv.Network,
		Stderr:  stderr,
		Env:     env,
		Dir:     filepath.Join(buildDir, drv.Source.RelativeBuildPath),
		Mounts:  mounts,
	}
	if opts.Shell {
		fmt.Printf("Opening shell for derivation %q\n", drv.Name)
		sbx.Args = []string{builderLocation}
		sbx.Stdin = os.Stdin
	}
	if err := sbx.Run(ctx); err != nil {
		return ExecError{Err: err, Logs: f}
	}
	return nil
}

type ExecError struct {
	Err  error
	Logs *os.File
}

func (err ExecError) Error() string {
	return err.Err.Error()
}

func (s *Store) hashAndMoveBuildOutputs(ctx context.Context, drv Derivation, outputPaths map[string]string, buildDir string) (outputs map[string]Output, err error) {
	var span trace.Span
	ctx, span = tracer.Start(ctx, "store.store.hashAndMoveBuildOutputs")
	defer span.End()
	outputs = map[string]Output{}
	for outputName, outputPath := range outputPaths {
		hshr := hasher.New()
		var reptarFile *os.File
		reptarFile, err = s.storeLengthTempFile()
		if err != nil {
			return
		}
		outputFolder := filepath.Base(outputPath)
		matches, err := s.archiveAndScanOutputDirectory(ctx, reptarFile, hshr, drv, outputFolder, buildDir)
		if err != nil {
			return nil, errors.Wrap(err, "error scanning output")
		}
		// remove build output, we have it in an archive
		if err = os.RemoveAll(outputPath); err != nil {
			return nil, errors.Wrap(err, "error removing build output")
		}

		hashedFolderName := hshr.String()

		// Nix adds the name to the output path but we are a
		// content-addressable-store so we remove so that derivations with
		// different names can share outputs
		newPath := s.joinStorePath(hashedFolderName)

		if !fileutil.PathExists(newPath) {
			if err := s.unarchiveAndReplaceOutputFolderName(
				ctx,
				reptarFile.Name(),
				newPath,
				outputFolder,
				hashedFolderName); err != nil {
				return nil, err
			}
		}
		if err := os.RemoveAll(reptarFile.Name()); err != nil {
			return nil, err
		}
		outputs[outputName] = Output{Path: hashedFolderName, Dependencies: matches}
		// logger.Print("Output at ", newPath)
	}
	return
}

func (s *Store) unarchiveAndReplaceOutputFolderName(ctx context.Context, archive, dst, outputFolder, hashedFolderName string) (err error) {
	var span trace.Span
	_, span = tracer.Start(ctx, "store.store.unarchiveAndReplaceOutputFolderName")
	defer span.End()
	pipeReader, pipWriter := io.Pipe()
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	errChan := make(chan error)
	doneChan := make(chan struct{})

	// Read the file and replace output folder names with the hashed folder name
	go func() {
		if _, err := textreplace.ReplaceBytes(
			f, pipWriter,
			[]byte(outputFolder), []byte(hashedFolderName),
		); err != nil {
			errChan <- err
			return
		}
		if err := pipWriter.Close(); err != nil {
			errChan <- err
			return
		}
	}()
	// Unarchive the resulting bytestream, pass the archive name because the lib
	// needs it to resolve name conflicts. TODO: this is probably an error,
	// wouldn't want the name of a random file to affect the tar output, need to
	// be sure this isn't causing any issues.
	go func() {
		tr := archiver.NewTar()
		// "archive" here is the name of the file that we open above
		if err := tr.UnarchiveReader(pipeReader, archive, dst); err != nil {
			errChan <- err
		}
		doneChan <- struct{}{}
	}()

	select {
	case err := <-errChan:
		return err
	case <-doneChan:
	}
	return
}

func (s *Store) archiveAndScanOutputDirectory(ctx context.Context, tarOutput, hashOutput io.Writer, drv Derivation, storeFolder, buildDir string) (
	matches []string, err error) {
	var span trace.Span
	_, span = tracer.Start(ctx, "store.store.archiveAndScanOutputDirectory")
	defer span.End()
	var storeValues []string

	for _, do := range drv.Dependencies {
		drv, found, err := s.LoadDerivation(do.Filename)
		if err != nil || !found {
			panic(fmt.Sprint(drv, err, do.Filename, found))
		}
		storeValues = append(storeValues,
			s.joinStorePath(drv.output(do.OutputName).Path),
		)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	errChan := make(chan error)
	resultChan := make(chan map[string]struct{})
	pipeReader, pipeWriter := io.Pipe()

	tarPipeReader, tarPipeWriter := io.Pipe()

	// write the output files into an archive
	go func() {
		btpw := bufio.NewWriter(tarPipeWriter)
		if err := reptar.Archive(s.joinStorePath(storeFolder), btpw); err != nil {
			errChan <- err
			return
		}
		_ = btpw.Flush()
		if err := tarPipeWriter.Close(); err != nil {
			errChan <- err
			return
		}
	}()

	// Replace any references to the build directory with fixed known string
	// value. Stream the output to both the hash bytestream and the archive
	// output stream now that the build path is replaced.
	go func() {
		defer func() {
			if err := pipeWriter.Close(); err != nil {
				errChan <- err
				return
			}
		}()
		if buildDir == "" {
			// TODO: remove this extra copy when we can?
			_, _ = io.Copy(io.MultiWriter(tarOutput, pipeWriter), tarPipeReader)
			return
		}
		bdBytes := []byte(buildDir)
		if _, err := textreplace.ReplaceBytes(
			tarPipeReader, io.MultiWriter(tarOutput, pipeWriter),
			bdBytes,
			append(
				// we need to copy the values out of the array so that the
				// previous byte reference isn't mutated
				append([]byte{}, bdBytes[:len(bdBytes)-32]...),
				bytes.Repeat([]byte("x"), 32)...,
			),
		); err != nil {
			errChan <- err
			return
		}
	}()

	pipeReader2, pipeWriter2 := io.Pipe()

	// In the hash bytes stream, replace all the bramble store path prefixes
	// with a known fixed value.
	go func() {
		_, matches, err := textreplace.ReplaceStringsPrefix(
			pipeReader, pipeWriter2, storeValues, s.StorePath,
			BramblePrefixOfRecord)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- matches
		if err := pipeWriter2.Close(); err != nil {
			errChan <- err
			return
		}
	}()

	// swap out references in the output to itself with null bytes so that
	// builds with a different randomly named build directory will still match
	// the hash of this one
	go func() {
		if _, err := textreplace.ReplaceBytes(
			pipeReader2, hashOutput,
			[]byte(storeFolder), bytes.Repeat([]byte{0}, len(storeFolder)),
		); err != nil {
			wg.Done()
			errChan <- errors.Wrap(err, "error replacing storeFolder with null bytes")
			return
		}
		wg.Done() // this is the end of the hash stream
	}()

	select {
	case err := <-errChan:
		return nil, err
	case result := <-resultChan:
		for match := range result {
			// remove prefix from dependency path
			match = strings.TrimPrefix(strings.Replace(match, s.StorePath, "", 1), "/")
			matches = append(matches, match)
		}
	}
	wg.Wait()
	select {
	// Check if there are any errors left in the chan
	case err := <-errChan:
		return nil, err
	default:
	}
	return
}

func (s *Store) hashNormalizedBuildOutput(location string, hash string) (err error) {
	pipeReader, pipeWriter := io.Pipe()
	errChan := make(chan error)
	resultChan := make(chan string)
	go func() {
		if err := reptar.Archive(location, pipeWriter); err != nil {
			errChan <- err
		}
		pipeWriter.Close()
	}()
	go func() {
		h := hasher.New()
		if _, err := textreplace.ReplaceBytes(
			pipeReader, h,
			[]byte(hash), bytes.Repeat([]byte{0}, len(hash)),
		); err != nil {
			errChan <- errors.Wrap(err, "error replacing self-reference outputs hash with with null bytes")
			return
		}
		resultChan <- h.String()
	}()

	select {
	case err := <-errChan:
		return err
	case result := <-resultChan:
		if result != hash {
			return errors.Errorf("Output hash %s doesn't match computed hash value %s", hash, result)
		}
		return nil
	}
}
