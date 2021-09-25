package bramblebuild

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/trace"
	"strings"
	"sync"
	"time"

	"github.com/certifi/gocertifi"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/src/logger"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/maxmcd/bramble/pkg/sandbox"
	"github.com/maxmcd/bramble/pkg/textreplace"
	"github.com/mholt/archiver/v3"
	"github.com/pkg/errors"
)

func (s *Store) NewBuilder(rootless bool, urlHashes map[string]string) *Builder {
	return &Builder{
		store:     s,
		URLHashes: urlHashes,
	}
}

type Builder struct {
	store     *Store
	rootless  bool
	URLHashes map[string]string
}

type BuildDerivationOptions struct {
	// ForceBuild will make sure we build even if the derivation already exists
	ForceBuild bool

	Shell bool
}

func (b *Builder) BuildDerivation(ctx context.Context, drv Derivation, opts BuildDerivationOptions) (builtDrv Derivation, didBuild bool, err error) {
	drv.InputDerivations = sortAndUniqueInputDerivations(drv.InputDerivations)
	drv = drv.makeConsistentNullJSONValues()

	exists, outputs, err := drv.populateOutputsFromStore()
	drv.Outputs = outputs
	if err != nil {
		return drv, false, err
	}
	filename := drv.Filename()
	logger.Debugw("buildDerivationIfNew", "derivation", filename, "exists", exists)
	if exists && !opts.ForceBuild {
		return drv, false, nil
	}
	logger.Print("Building derivation", filename)
	logger.Debugw(drv.PrettyJSON())
	if drv, err = b.buildDerivation(ctx, drv, opts.Shell); err != nil {
		return drv, false, errors.Wrap(err, "error building "+filename)
	}
	// TODO: lock store on write
	return drv, true, b.store.WriteDerivation(drv)
}

func (b *Builder) buildDerivation(ctx context.Context, drv Derivation, shell bool) (Derivation, error) {
	var err error
	var task *trace.Task
	ctx, task = trace.NewTask(ctx, "buildDerivation")
	defer task.End()

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

	if shell && (drv.Builder == "fetch_url" || drv.Builder == "fetch_git") {
		return drv, errors.New("can't spawn a shell with a builtin builder")
	}

	switch drv.Builder {
	case "fetch_url":
		err = b.fetchURLBuilder(ctx, drvCopy, outputPaths)
	case "fetch_git":
		err = b.fetchGitBuilder(ctx, drvCopy, outputPaths)
	default:
		err = b.regularBuilder(ctx, drvCopy, buildDir, outputPaths, shell)
	}
	if err != nil {
		return drv, err
	}

	if err := os.RemoveAll(buildDir); err != nil {
		return drv, err
	}

	var outputs map[string]Output

	if drv.Builder == "fetch_url" {
		// fetch url just hashes the directory and moves it into the output
		// location, no archiving and byte replacing
		outputs, err = b.hashAndMoveFetchURL(ctx, drv, outputPaths["out"])
	} else {
		outputs, err = b.store.hashAndMoveBuildOutputs(ctx, drv, outputPaths, buildDir)
		err = errors.Wrap(err, "hash and move build outputs") // noop if err is nil
	}
	if err != nil {
		return drv, err
	}

	drv.Outputs, err = outputsToOutput(drv.OutputNames, outputs)
	return drv, err
}

func (b *Builder) hashAndMoveFetchURL(ctx context.Context, drv Derivation, outputPath string) (outputs map[string]Output, err error) {
	region := trace.StartRegion(ctx, "hashAndMoveFetchUrl")
	defer region.End()

	hshr := hasher.NewHasher()
	_, err = b.store.archiveAndScanOutputDirectory(ctx, ioutil.Discard, hshr, drv, filepath.Base(outputPath), "")
	if err != nil {
		return nil, err
	}
	outputFolderName := hshr.String()
	outputs = map[string]Output{"out": {Path: outputFolderName}}
	outputStorePath := b.store.joinStorePath(outputFolderName)
	if fileutil.PathExists(outputStorePath) {
		err = os.RemoveAll(outputPath)
	} else {
		err = os.Rename(outputPath, outputStorePath)
	}
	if err == nil {
		logger.Print("Output at", outputStorePath)
	}
	return outputs, err
}

func (b *Builder) fetchGitBuilder(ctx context.Context, drv Derivation, outputPaths map[string]string) (err error) {
	outputPath, ok := outputPaths["out"]
	if len(outputPaths) > 1 || !ok {
		return errors.New("the fetch_url builder can only have the defalt output \"out\"")
	}
	url, ok := drv.Env["url"]
	if !ok {
		return errors.New("fetch_url requires the environment variable 'url' to be set")
	}
	// derivation can provide a hash, but usually this is just in the lockfile
	hash := drv.Env["hash"]

	if err := b.store.runGit(ctx, RunDerivationOptions{
		Mounts: []string{outputPath},
		Args:   []string{"git", "clone", url, outputPath},
		Dir:    outputPath,
	}); err != nil {
		return err
	}

	_ = hash
	return nil
}

func (b *Builder) fetchURLBuilder(ctx context.Context, drv Derivation, outputPaths map[string]string) (err error) {
	region := trace.StartRegion(ctx, "fetchURLBuilder")
	defer region.End()

	if _, ok := outputPaths["out"]; len(outputPaths) > 1 || !ok {
		return errors.New("the fetch_url builder can only have the defalt output \"out\"")
	}
	url, ok := drv.Env["url"]
	if !ok {
		return errors.New("fetch_url requires the environment variable 'url' to be set")
	}
	// derivation can provide a hash, but usually this is just in the lockfile
	hash := drv.Env["hash"]
	path, err := b.downloadFile(ctx, url, hash)
	if err != nil {
		return err
	}
	// TODO: what if this package changes?
	if err = archiver.Unarchive(path, outputPaths["out"]); err != nil {
		if !strings.Contains(err.Error(), "format unrecognized by filename") {
			return errors.Wrap(err, "error unpacking url archive")
		}
		return os.Rename(path, filepath.Join(outputPaths["out"], filepath.Base(url)))
	}
	return nil
}

// downloadFile downloads a file into the store. Must include an expected hash
// of the downloaded file as a hex string of a sha256 hash
func (b *Builder) downloadFile(ctx context.Context, url string, hash string) (path string, err error) {
	logger.Printfln("Downloading url %s", url)
	if hash != "" {
		byt, err := hex.DecodeString(hash)
		if err != nil {
			err = errors.Wrap(err, fmt.Sprintf("error decoding hash %q; is it hexadecimal?", hash))
			return "", err
		}
		storePrefixHash := hasher.BytesToBase32Hash(byt)
		matches, err := filepath.Glob(b.store.joinStorePath(storePrefixHash) + "*")
		if err != nil {
			err = errors.Wrap(err, "error searching for existing hashed content")
			return "", err
		}
		if len(matches) != 0 {
			return matches[0], nil
		}
	}

	// TODO: must pass url hashes
	existingHash, exists := b.URLHashes[url]
	if exists && hash != "" && hash != existingHash {
		return "", errors.Errorf("when downloading the file %q a hash %q was provided in"+
			" code but the hash %q was in the lock file, exiting", url, hash, existingHash)
	}

	// if we don't have a hash to validate, validate the one we already have
	if hash == "" && exists {
		hash = existingHash
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
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
		return "", errors.Errorf("Unexpected http status code %d when fetching url %q", resp.StatusCode, url)
	}
	defer resp.Body.Close()
	contentHash, path, err := b.store.writeReader(resp.Body, filepath.Base(url), hash)
	if err == hasher.ErrHashMismatch {
		err = errors.Errorf(
			"Got incorrect hash for url %s.\nwanted %q\ngot    %q",
			url, hash, contentHash)
	} else if err != nil {
		return
	}
	//REFAC: Is it ok to not keep track of what has been added???
	b.URLHashes[url] = contentHash
	return path, nil
}

func (b *Builder) regularBuilder(ctx context.Context, drv Derivation, buildDir string,
	outputPaths map[string]string, shell bool) (err error) {
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
	if b.rootless {
		cmd := exec.Cmd{
			Path:   builderLocation,
			Args:   append([]string{builderLocation}, drv.Args...),
			Stdout: os.Stdout,
			Stderr: os.Stderr,
			Dir:    filepath.Join(buildDir, drv.Source.RelativeBuildPath),
			Env:    env,
		}
		return cmd.Run()
	}
	sbx := sandbox.Sandbox{
		Args:   append([]string{builderLocation}, drv.Args...),
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Env:    env,
		Dir:    filepath.Join(buildDir, drv.Source.RelativeBuildPath),
		Mounts: mounts,
	}
	if shell {
		fmt.Printf("Opening shell for derivation %q\n", drv.Name)
		sbx.Args = []string{builderLocation}
		sbx.Stdin = os.Stdin
	}
	return sbx.Run(ctx)
}

func (s *Store) hashAndMoveBuildOutputs(ctx context.Context, drv Derivation, outputPaths map[string]string, buildDir string) (outputs map[string]Output, err error) {
	// if drv.Name == "self-reference" {
	// 	fmt.Println(drv.PrettyJSON(), outputPaths)
	// 	panic("")
	// }
	fmt.Println(outputPaths)
	region := trace.StartRegion(ctx, "hashAndMoveBuildOutputs")
	defer region.End()

	outputs = map[string]Output{}
	for outputName, outputPath := range outputPaths {
		hshr := hasher.NewHasher()
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
		logger.Print("Output at ", newPath)
	}
	return
}
func (s *Store) unarchiveAndReplaceOutputFolderName(ctx context.Context, archive, dst, outputFolder, hashedFolderName string) (err error) {
	region := trace.StartRegion(ctx, "unarchiveAndReplaceOutputFolderName")
	defer region.End()
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
	region := trace.StartRegion(ctx, "archiveAndScanOutputDirectory")
	defer region.End()
	var storeValues []string

	for _, do := range drv.InputDerivations {
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
		if err := reptar.Reptar(s.joinStorePath(storeFolder), tarPipeWriter); err != nil {
			errChan <- err
			return
		}
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
