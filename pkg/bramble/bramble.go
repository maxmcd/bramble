package bramble

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime/debug"
	"runtime/trace"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/certifi/gocertifi"
	git "github.com/go-git/go-git/v5"
	gitclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/hashicorp/terraform/dag"
	"github.com/hashicorp/terraform/tfdiags"
	"github.com/mholt/archiver/v3"
	"github.com/pkg/errors"
	"go.starlark.net/repl"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"

	"github.com/maxmcd/bramble/pkg/assert"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/maxmcd/bramble/pkg/sandbox"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/maxmcd/bramble/pkg/store"
	"github.com/maxmcd/bramble/pkg/textreplace"
)

func init() {
	// It's easier to start giving away free coffee than it is to take away
	// free coffee.

	// I think this would allow storing arbitrary state in function closures
	// and make the codebase much harder to reason about. Maybe we want this
	// level of complexity at some point, but nice to avoid for now.
	resolve.AllowLambda = false
	resolve.AllowNestedDef = false

	// Recursion might make it easier to write long executing code.
	resolve.AllowRecursion = false

	// Sets seem harmless tho?
	resolve.AllowSet = true

	// See little need for this (currently), but open to allowing it. Are there
	// correctness issues here?
	resolve.AllowFloat = false
}

var (
	BrambleExtension string = ".bramble"
)

type Bramble struct {
	thread      *starlark.Thread
	predeclared starlark.StringDict

	config         Config
	configLocation string
	lockFile       LockFile
	lockFileLock   sync.Mutex

	derivationFn *DerivationFunction
	// derivations that are touched by running commands
	inputDerivations DerivationOutputs

	store store.Store

	moduleCache   map[string]string
	filenameCache *BiStringMap
	importGraph   *AcyclicGraph

	derivations *DerivationsMap
}

func (b *Bramble) LoadDerivation(filename string) (drv *Derivation, exists bool, err error) {
	fileLocation := b.store.JoinStorePath(filename)
	_, err = os.Stat(fileLocation)
	if err != nil {
		return nil, false, nil
	}
	file, err := os.Open(fileLocation)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = file.Close() }()
	drv = &Derivation{}
	return drv, true, json.NewDecoder(file).Decode(drv)
}
func (b *Bramble) checkForExistingDerivation(filename string) (outputs []Output, exists bool, err error) {
	existingDrv, exists, err := b.LoadDerivation(filename)
	// It doesn't exist if it doesn't exist
	if !exists {
		return nil, exists, err
	}
	// It doesn't exist if it doesn't have the outputs we need
	return existingDrv.Outputs, !existingDrv.MissingOutput(), err
}

func (b *Bramble) buildDerivationIfNew(ctx context.Context, drv *Derivation) (err error) {
	filename := drv.filename()
	outputs, exists, err := b.checkForExistingDerivation(filename)
	if err != nil {
		return err
	}
	if exists {
		drv.Outputs = outputs
		b.derivations.Set(filename, drv)
		return
	}
	logger.Print("Building derivation", filename)
	logger.Debugw(drv.prettyJSON())

	if err = b.buildDerivation(ctx, drv, false); err != nil {
		return errors.Wrap(err, "error building "+filename)
	}
	return b.writeDerivation(drv)
}

func (b *Bramble) hashAndMoveFetchURL(ctx context.Context, drv *Derivation, outputPath string) (err error) {
	region := trace.StartRegion(ctx, "hashAndMoveFetchUrl")
	defer region.End()

	hshr := hasher.NewHasher()
	_, err = b.archiveAndScanOutputDirectory(ctx, ioutil.Discard, hshr, drv, filepath.Base(outputPath))
	if err != nil {
		return err
	}
	outputFolderName := hshr.String()
	drv.SetOutput("out", Output{Path: outputFolderName})
	outputStorePath := b.store.JoinStorePath(outputFolderName)
	if fileutil.PathExists(outputStorePath) {
		err = os.RemoveAll(outputPath)
	} else {
		err = os.Rename(outputPath, outputStorePath)
	}
	if err == nil {
		logger.Print("Output at", outputStorePath)
	}
	return
}

func (b *Bramble) buildDerivation(ctx context.Context, drv *Derivation, shell bool) (err error) {
	var task *trace.Task
	ctx, task = trace.NewTask(ctx, "buildDerivation")
	defer task.End()

	buildDir, err := b.store.TempDir()
	if err != nil {
		return
	}
	if drv.BuildContextSource != "" {
		if err = fileutil.CopyDirectory(b.store.JoinStorePath(drv.BuildContextSource), buildDir); err != nil {
			err = errors.Wrap(err, "error copying sources into build dir")
			return
		}
	}
	outputPaths := map[string]string{}
	for _, name := range drv.OutputNames {
		// TODO: use directory within store instead so that we can rewrite self-referential paths
		if outputPaths[name], err = b.store.TempDir(); err != nil {
			return
		}
	}
	drvCopy, err := b.copyDerivationWithOutputValuesReplaced(drv)
	if err != nil {
		return
	}

	if shell && (drv.Builder == "fetch_url" || drv.Builder == "fetch_git") {
		return errors.New("can't spawn a shell with a builtin builder")
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
		return
	}

	if drv.Builder == "fetch_url" {
		// fetch url just hashes the directory and moves it into the output
		// location, no archiving and byte replacing
		return b.hashAndMoveFetchURL(ctx, drv, outputPaths["out"])
	}
	return errors.Wrap(b.hashAndMoveBuildOutputs(ctx, drv, outputPaths), "hash and move build outputs")
}

func (b *Bramble) hashAndMoveBuildOutputs(ctx context.Context, drv *Derivation, outputPaths map[string]string) (err error) {
	region := trace.StartRegion(ctx, "hashAndMoveBuildOutputs")
	defer region.End()

	for outputName, outputPath := range outputPaths {
		hshr := hasher.NewHasher()
		var reptarFile *os.File
		reptarFile, err = b.createTmpFile()
		if err != nil {
			return
		}
		outputFolder := filepath.Base(outputPath)
		matches, err := b.archiveAndScanOutputDirectory(ctx, reptarFile, hshr, drv, outputFolder)
		if err != nil {
			return errors.Wrap(err, "error scanning output")
		}
		// remove build output, we have it in an archive
		if err = os.RemoveAll(outputPath); err != nil {
			return errors.Wrap(err, "error removing build output")
		}

		hashedFolderName := hshr.String()

		// Nix adds the name to the output path but we are a
		// content-addressable-store so we remove so that derivations with
		// different names can share outputs
		newPath := b.store.JoinStorePath(hashedFolderName)

		if !fileutil.PathExists(newPath) {
			if err := b.unarchiveAndReplaceOutputFolderName(
				ctx,
				reptarFile.Name(),
				newPath,
				outputFolder,
				hashedFolderName); err != nil {
				return err
			}
		}
		if err := os.RemoveAll(reptarFile.Name()); err != nil {
			return err
		}

		drv.SetOutput(outputName, Output{Path: hashedFolderName, Dependencies: matches})
		logger.Print("Output at ", newPath)
	}
	return nil
}

func (b *Bramble) unarchiveAndReplaceOutputFolderName(ctx context.Context, archive, dst, outputFolder, hashedFolderName string) (err error) {
	region := trace.StartRegion(ctx, "unarchiveAndReplaceOutputFolderName")
	defer region.End()
	pipeReader, pipWriter := io.Pipe()
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	errChan := make(chan error)
	doneChan := make(chan struct{})

	go func() {
		if _, err := textreplace.ReplaceBytes(
			f, pipWriter,
			[]byte(outputFolder), []byte(hashedFolderName)); err != nil {
			errChan <- err
			return
		}
		if err := pipWriter.Close(); err != nil {
			errChan <- err
			return
		}
	}()
	go func() {
		tr := archiver.NewTar()
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

func (b *Bramble) archiveAndScanOutputDirectory(ctx context.Context, tarOutput, hashOutput io.Writer, drv *Derivation, storeFolder string) (
	matches []string, err error) {
	region := trace.StartRegion(ctx, "archiveAndScanOutputDirectory")
	defer region.End()
	var storeValues []string
	oldStorePath := b.store.StorePath

	for _, do := range drv.InputDerivations {
		storeValues = append(storeValues,
			b.store.JoinStorePath(
				b.derivations.Get(do.Filename).Output(do.OutputName).Path,
			),
		)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	errChan := make(chan error)
	resultChan := make(chan map[string]struct{})
	pipeReader, pipeWriter := io.Pipe()
	pipeReader2, pipeWriter2 := io.Pipe()

	// write the output files into an archive
	go func() {
		if err := reptar.Reptar(b.store.JoinStorePath(storeFolder), io.MultiWriter(tarOutput, pipeWriter)); err != nil {
			errChan <- err
			return
		}
		if err := pipeWriter.Close(); err != nil {
			errChan <- err
			return
		}
	}()

	// replace all the bramble store path prefixes with a known fixed value
	// also write this byte stream out as a tar to unpack later as the final output
	go func() {
		new := BramblePrefixOfRecord
		_, matches, err := textreplace.ReplaceStringsPrefix(
			pipeReader, pipeWriter2, storeValues, oldStorePath, new)
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
	// builds with a different randomly named build directory will still
	// match the hash of this one
	go func() {
		if _, err := textreplace.ReplaceBytes(
			pipeReader2, hashOutput,
			[]byte(storeFolder), make([]byte, len(storeFolder)),
		); err != nil {
			errChan <- err
			return
		}
		wg.Done()
	}()

	select {
	case err := <-errChan:
		return nil, err
	case result := <-resultChan:
		for match := range result {
			// remove prefix from dependency path
			match = strings.TrimPrefix(strings.Replace(match, oldStorePath, "", 1), "/")
			matches = append(matches, match)
		}
	}
	wg.Wait()
	return
}

func (b *Bramble) moduleNameFromFileName(filename string) (moduleName string, err error) {
	filename, err = filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	if !fileutil.FileExists(filename) {
		return "", errors.Errorf("bramble file %q doesn't exist", filename)
	}
	if !strings.HasPrefix(filename, b.configLocation) {
		return "", errors.New("we don't support external modules yet")
	}
	relativeWorkspacePath, err := filepath.Rel(b.configLocation, filename)
	if err != nil {
		return "", err
	}
	moduleName = filepath.Join("github.com/maxmcd/bramble", relativeWorkspacePath)
	moduleName = strings.TrimSuffix(moduleName, "/default"+BrambleExtension)
	moduleName = strings.TrimSuffix(moduleName, BrambleExtension)
	return
}

// setDerivationBuilder is used during instantiation to set various attributes on the
// derivation for a specific builder
func (b *Bramble) setDerivationBuilder(drv *Derivation, builder starlark.Value) (err error) {
	switch v := builder.(type) {
	case starlark.String:
		drv.Builder = v.GoString()
	default:
		return errors.Errorf("no builder for %q", builder.Type())
	}
	return
}

func (b *Bramble) createTmpFile() (f *os.File, err error) {
	return ioutil.TempFile(b.store.StorePath, BuildDirPattern)
}

func (b *Bramble) writeDerivation(drv *Derivation) error {
	filename := drv.filename()
	fileLocation := b.store.JoinStorePath(filename)

	return ioutil.WriteFile(fileLocation, drv.JSON(), 0644)
}

func (b *Bramble) fetchGitBuilder(ctx context.Context, drv *Derivation, outputPaths map[string]string) (err error) {
	region := trace.StartRegion(ctx, "fetchGitBuilder")
	defer region.End()

	certPool, err := gocertifi.CACerts()
	customClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: certPool},
		},
	}

	// Override http(s) default protocol to use our custom client
	gitclient.InstallProtocol("https", githttp.NewClient(customClient))
	_, _ = git.PlainClone("", false, &git.CloneOptions{})

	if _, ok := outputPaths["out"]; len(outputPaths) > 1 || !ok {
		return errors.New("the fetchurl builtin can only have the defalt output \"out\"")
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
		return errors.Wrap(err, "error unpacking url archive")
	}
	return nil
}

func (b *Bramble) fetchURLBuilder(ctx context.Context, drv *Derivation, outputPaths map[string]string) (err error) {
	region := trace.StartRegion(ctx, "fetchURLBuilder")
	defer region.End()

	if _, ok := outputPaths["out"]; len(outputPaths) > 1 || !ok {
		return errors.New("the fetchurl builtin can only have the defalt output \"out\"")
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

// Keep this, we'll need it at some point
func (b *Bramble) dockerRegularBuilder(ctx context.Context, drv *Derivation, buildDir string, outputPaths map[string]string) (err error) {
	region := trace.StartRegion(ctx, "dockerRegularBuilder")
	defer region.End()

	builderLocation := drv.Builder
	if _, err := os.Stat(builderLocation); err != nil {
		return errors.Wrap(err, "error checking if builder location exists")
	}

	options := runDockerBuildOptions{
		buildDir:    buildDir,
		outputPaths: map[string]string{},
		env:         drv.env(),
		cmd:         append([]string{builderLocation}, drv.Args...),
		workingDir:  filepath.Join(buildDir, drv.BuildContextRelativePath),
	}

	for outputName, outputPath := range outputPaths {
		options.env = append(options.env, fmt.Sprintf("%s=%s", outputName, outputPath))
		options.outputPaths[outputName] = outputPath
	}

	return b.runDockerBuild(ctx, drv.filename(), options)
}

func (b *Bramble) regularBuilder(ctx context.Context, drv *Derivation, buildDir string,
	outputPaths map[string]string, shell bool) (err error) {
	builderLocation := drv.Builder
	if _, err := os.Stat(builderLocation); err != nil {
		return errors.Wrap(err, "builder location doesn't exist")
	}
	env := drv.env()
	mounts := []string{
		b.store.StorePath + ":ro",
		buildDir,
		// "/dev/", //TODO: this can't be allowed
	}
	for outputName, outputPath := range outputPaths {
		env = append(env, fmt.Sprintf("%s=%s", outputName, outputPath))
		mounts = append(mounts, outputPath)
	}
	chrootDir, err := ioutil.TempDir("", "bramble-chroot-")
	// chrootDir, err := b.store.TempBuildDir()
	if err != nil {
		return err
	}
	u, err := user.Current()
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	sbx := sandbox.Sandbox{
		Path:       builderLocation,
		Args:       drv.Args,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		UserID:     uid,
		GroupID:    gid,
		Env:        env,
		ChrootPath: chrootDir,
		Dir:        filepath.Join(buildDir, drv.BuildContextRelativePath),
		Mounts:     mounts,
	}
	if shell {
		sbx.Args = nil
		sbx.Stdin = os.Stdin
	}
	return sbx.Run(ctx)
}

// downloadFile downloads a file into the store. Must include an expected hash
// of the downloaded file as a hex string of a  sha256 hash
func (b *Bramble) downloadFile(ctx context.Context, url string, hash string) (path string, err error) {
	logger.Printfln("Downloading url %s", url)
	if hash != "" {
		byt, err := hex.DecodeString(hash)
		if err != nil {
			err = errors.Wrap(err, fmt.Sprintf("error decoding hash %q; is it hexadecimal?", hash))
			return "", err
		}
		storePrefixHash := hasher.BytesToBase32Hash(byt)
		matches, err := filepath.Glob(b.store.JoinStorePath(storePrefixHash) + "*")
		if err != nil {
			err = errors.Wrap(err, "error searching for existing hashed content")
			return "", err
		}
		if len(matches) != 0 {
			return matches[0], nil
		}
	}

	existingHash, exists := b.lockFile.URLHashes[url]
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
	defer resp.Body.Close()
	contentHash, path, err := b.store.WriteReader(resp.Body, filepath.Base(url), hash)
	if err == hasher.ErrHashMismatch {
		err = errors.Errorf(
			"Got incorrect hash for url %s.\nwanted %q\ngot    %q",
			url, hash, contentHash)
	} else if err != nil {
		return
	}
	return path, b.addURLHashToLockfile(url, contentHash)
}

func (b *Bramble) calculateDerivationInputSources(ctx context.Context, drv *Derivation) (err error) {
	region := trace.StartRegion(ctx, "calculateDerivationInputSources")
	defer region.End()

	if len(drv.sources) == 0 {
		return
	}
	tmpDir, err := b.store.TempDir()
	if err != nil {
		return
	}

	sources := drv.sources
	drv.sources = []string{}
	absDir, err := filepath.Abs(drv.location)
	if err != nil {
		return
	}
	// get absolute paths for all sources
	for i, src := range sources {
		sources[i] = filepath.Join(absDir, src)
	}
	prefix := fileutil.CommonFilepathPrefix(append(sources, absDir))
	relBramblefileLocation, err := filepath.Rel(prefix, absDir)
	if err != nil {
		return
	}

	if err = fileutil.CopyFilesByPath(prefix, sources, tmpDir); err != nil {
		return
	}
	// sometimes the location the derivation runs from is not present
	// in the structure of the copied source files. ensure that we add it
	runLocation := filepath.Join(tmpDir, relBramblefileLocation)
	if err = os.MkdirAll(runLocation, 0755); err != nil {
		return
	}

	hshr := hasher.NewHasher()
	if err = reptar.Reptar(tmpDir, hshr); err != nil {
		return
	}
	storeLocation := b.store.JoinStorePath(hshr.String())
	if fileutil.PathExists(storeLocation) {
		if err = os.RemoveAll(tmpDir); err != nil {
			return
		}
	} else {
		if err = os.Rename(tmpDir, storeLocation); err != nil {
			return
		}
	}
	drv.BuildContextSource = hshr.String()
	drv.BuildContextRelativePath = relBramblefileLocation
	drv.SourcePaths = append(drv.SourcePaths, hshr.String())
	sort.Strings(drv.SourcePaths)
	return
}

func (b *Bramble) stringsReplacerForOutputs(outputs DerivationOutputs) (replacer *strings.Replacer, err error) {
	// Find all the replacements we need to make, template strings need to
	// become filesystem paths
	replacements := []string{}
	for _, do := range outputs {
		d := b.derivations.Get(do.Filename)
		if d == nil {
			return nil, errors.Errorf(
				"couldn't find a derivation with the filename %q in our cache. have we built it yet?", do.Filename)
		}
		path := filepath.Join(
			b.store.StorePath,
			d.Output(do.OutputName).Path,
		)
		replacements = append(replacements, do.templateString(), path)
	}
	// Replace the content using the json body and then convert it back into a
	// new derivation
	return strings.NewReplacer(replacements...), nil
}

func (b *Bramble) copyDerivationWithOutputValuesReplaced(drv *Derivation) (copy *Derivation, err error) {
	// Find all derivation output template strings within the derivation
	outputs := drv.searchForDerivationOutputs()

	replacer, err := b.stringsReplacerForOutputs(outputs)
	if err != nil {
		return
	}
	replacedJSON := replacer.Replace(string(drv.JSON()))
	err = json.Unmarshal([]byte(replacedJSON), &copy)
	copy.location = drv.location
	return copy, err
}

func (b *Bramble) derivationsToDerivationOutputs(drvs []*Derivation) (dos DerivationOutputs) {
	for _, drv := range drvs {
		filename := drv.filename()
		for _, name := range drv.OutputNames {
			dos = append(dos, DerivationOutput{Filename: filename, OutputName: name})
		}
	}
	return dos
}

func (b *Bramble) assembleDerivationDependencyGraph(dos DerivationOutputs) *AcyclicGraph {
	graph := NewAcyclicGraph()
	_ = graph
	root := "root"
	graph.Add(root)
	var processDO func(do DerivationOutput)
	processDO = func(do DerivationOutput) {
		drv := b.derivations.Get(do.Filename)
		if drv == nil {
			panic(do)
		}
		for _, inputDO := range drv.InputDerivations {
			graph.Add(inputDO)
			graph.Connect(dag.BasicEdge(do, inputDO))
			processDO(inputDO) // TODO, not recursive
		}
	}
	for _, do := range dos {
		graph.Add(do)
		graph.Connect(dag.BasicEdge(root, do))
		processDO(do)
	}
	return graph
}

func (b *Bramble) buildDerivationOutputs(ctx context.Context, dos DerivationOutputs, skipDerivation *Derivation) (err error) {
	graph := b.assembleDerivationDependencyGraph(dos)
	var wg sync.WaitGroup
	errChan := make(chan error)
	semaphore := make(chan struct{}, 1)
	var errored bool
	if err = graph.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		graph.Walk(func(v dag.Vertex) (_ tfdiags.Diagnostics) {
			if errored {
				return
			}
			semaphore <- struct{}{}
			defer func() {
				<-semaphore
			}()
			// serial for now

			if root, ok := v.(string); ok && root == "root" {
				return
			}
			do := v.(DerivationOutput)
			drv := b.derivations.Get(do.Filename)
			if drv.Output(do.OutputName).Path != "" {
				return
			}
			if skipDerivation != nil && skipDerivation.filename() == drv.filename() {
				return
			}
			wg.Add(1)
			if err := b.buildDerivationIfNew(ctx, drv); err != nil {
				// Passing the error might block, so we need an explicit Done
				// call here.
				wg.Done()
				errored = true
				logger.Print(err)
				errChan <- err
				return
			}
			wg.Done()
			return
		})
		errChan <- nil
	}()
	err = <-errChan
	cancel() // Call cancel on the context, no-op if nothing is running
	if err != nil {
		// If we receive an error cancel the context and wait for any jobs that
		// are running.
		wg.Wait()
	}
	return err
}

func (b *Bramble) init() (err error) {
	if b.configLocation != "" {
		return errors.New("can't initialize Bramble twice")
	}

	b.moduleCache = map[string]string{}
	b.filenameCache = NewBiStringMap()
	b.derivations = &DerivationsMap{}
	b.importGraph = NewAcyclicGraph()

	if b.store, err = store.NewStore(); err != nil {
		return err
	}

	// ensures we have a bramble.toml in the current or parent dir
	if b.config, b.lockFile, b.configLocation, err = findConfig(); err != nil {
		return err
	}

	b.thread = &starlark.Thread{
		Name: "main",
		Load: b.load,
	}

	return b.initPredeclared()
}

func (b *Bramble) initPredeclared() (err error) {
	if b.derivationFn != nil {
		return errors.New("can't init predeclared twice")
	}
	// creates the derivation function and checks we have a valid bramble path and store
	b.derivationFn, err = NewDerivationFunction(b)
	if err != nil {
		return
	}

	assertGlobals, err := assert.LoadAssertModule()
	if err != nil {
		return
	}
	// set the necessary error reporter so that the assert package can catch
	// errors
	assert.SetReporter(b.thread, runErrorReporter{})

	b.predeclared = starlark.StringDict{
		"derivation": b.derivationFn,
		"assert":     assertGlobals["assert"],
	}
	return
}

func (b *Bramble) load(thread *starlark.Thread, module string) (globals starlark.StringDict, err error) {
	thisFilesModuleName, _ := b.filenameCache.Load(thread.CallFrame(0).Pos.Filename())
	b.importGraph.Add(thisFilesModuleName)
	b.importGraph.Add(module)
	b.importGraph.Connect(dag.BasicEdge(module, thisFilesModuleName))
	globals, err = b.resolveModule(module)
	return
}

func (b *Bramble) execTestFileContents(script string) (v starlark.Value, err error) {
	globals, err := starlark.ExecFile(b.thread, ".bramble", script, b.predeclared)
	if err != nil {
		return nil, err
	}
	return starlark.Call(b.thread, globals["test"], nil, nil)
}

func findBrambleFiles(path string) (brambleFiles []string, err error) {
	if fileutil.FileExists(path) {
		return []string{path}, nil
	}
	if fileutil.FileExists(path + BrambleExtension) {
		return []string{path + BrambleExtension}, nil
	}
	return brambleFiles, filepath.Walk(path, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(fi.Name()) != BrambleExtension {
			return nil
		}
		brambleFiles = append(brambleFiles, path)
		return nil
	})
}

// runErrorReporter reports errors during a run. These errors are just passed up the thread
type runErrorReporter struct{}

func (e runErrorReporter) Error(err error) {}
func (e runErrorReporter) FailNow() bool   { return true }

func (b *Bramble) compileStarlarkPath(path string) (prog *starlark.Program, err error) {
	compiledProgram, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "error opening moduleCache storeLocation")
	}
	return starlark.CompiledProgram(compiledProgram)
}

func (b *Bramble) sourceStarlarkProgram(moduleName, filename string) (prog *starlark.Program, err error) {
	b.filenameCache.Store(filename, moduleName)
	storeLocation, ok := b.moduleCache[moduleName]
	if ok {
		// we have a cached binary location in the cache map, so we just use that
		return b.compileStarlarkPath(b.store.JoinStorePath(storeLocation))
	}

	// hash the file input
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	hshr := hasher.NewHasher()
	if _, err = io.Copy(hshr, f); err != nil {
		return nil, err
	}
	inputHash := hshr.String()

	inputHashStoreLocation := b.store.JoinBramblePath("var", "star-cache", inputHash)
	storeLocation, ok = fileutil.ValidSymlinkExists(inputHashStoreLocation)
	if ok {
		// if we have the hashed input on the filesystem cache and it points to a valid path
		// in the store, use that store path and add the cached location to the map
		relStoreLocation, err := filepath.Rel(b.store.StorePath, storeLocation)
		if err != nil {
			return nil, err
		}
		b.moduleCache[moduleName] = relStoreLocation
		return b.compileStarlarkPath(relStoreLocation)
	}

	// if we're this far we don't have a cache of the program, process it directly
	if _, err = f.Seek(0, 0); err != nil {
		return
	}
	_, prog, err = starlark.SourceProgram(filename, f, b.predeclared.Has)
	if err != nil {
		return
	}

	var buf bytes.Buffer
	if err = prog.Write(&buf); err != nil {
		return nil, err
	}
	_, path, err := b.store.WriteReader(&buf, filepath.Base(filename), "")
	if err != nil {
		return
	}
	b.moduleCache[moduleName] = filepath.Base(path)
	_ = os.Remove(inputHashStoreLocation)
	return prog, os.Symlink(path, inputHashStoreLocation)
}

func (b *Bramble) starlarkExecFile(moduleName, filename string) (globals starlark.StringDict, err error) {
	prog, err := b.sourceStarlarkProgram(moduleName, filename)
	if err != nil {
		return
	}
	g, err := prog.Init(b.thread, b.predeclared)
	g.Freeze()
	return g, err
}

func (b *Bramble) repl(_ []string) (err error) {
	if err := b.init(); err != nil {
		return err
	}
	repl.REPL(b.thread, b.predeclared)
	return nil
}

func (b *Bramble) parseBuildArg(cmd string, args []string) (module, fn string, derivations []*Derivation, err error) {
	if len(args) == 0 {
		logger.Printfln(`"bramble %s" requires 1 argument`, cmd)
		err = flag.ErrHelp
		return
	}

	if err = b.init(); err != nil {
		return
	}

	// parse something like ./tests:foo into the correct module and function
	// name
	if module, fn, err = b.parseModuleFuncArgument(args); err != nil {
		return
	}

	// parse the module and all of its imports, return available functions
	globals, err := b.resolveModule(module)
	if err != nil {
		return
	}
	toCall, ok := globals[fn]
	if !ok {
		err = errors.Errorf("function %q not found in module %q", fn, module)
		return
	}

	values, err := starlark.Call(&starlark.Thread{}, toCall, nil, nil)
	if err != nil {
		err = errors.Wrap(err, "error running")
		return
	}

	// The function must return a single derivation or a list of derivations, or
	// a tuple of derivations. We turn them into an array.
	derivations = valuesToDerivations(values)
	return
}

func (b *Bramble) shell(ctx context.Context, args []string) (err error) {
	module, fn, derivations, err := b.parseBuildArg("build", args)
	if err != nil {
		return err
	}
	if len(derivations) > 1 {
		return errors.New(`cannot run "bramble shell" with a function that returns multiple derivations`)
	}
	shellDerivation := derivations[0]

	if err = b.buildDerivationOutputs(ctx, b.derivationsToDerivationOutputs(derivations), shellDerivation); err != nil {
		return
	}

	if err := b.writeConfigMetadata(derivations, module, fn); err != nil {
		return err
	}
	filename := shellDerivation.filename()
	logger.Print("Launching shell for derivation", filename)
	logger.Debugw(shellDerivation.prettyJSON())
	if err = b.buildDerivation(ctx, shellDerivation, true); err != nil {
		return errors.Wrap(err, "error spawning "+filename)
	}
	return nil
}

func (b *Bramble) build(ctx context.Context, args []string) (err error) {
	module, fn, derivations, err := b.parseBuildArg("build", args)
	if err != nil {
		return err
	}
	if err = b.buildDerivationOutputs(ctx, b.derivationsToDerivationOutputs(derivations), nil); err != nil {
		return
	}

	return b.writeConfigMetadata(derivations, module, fn)
}

func valuesToDerivations(values starlark.Value) (derivations []*Derivation) {
	switch v := values.(type) {
	case *Derivation:
		return []*Derivation{v}
	case *starlark.List:
		for _, v := range starutil.ListToValueList(v) {
			derivations = append(derivations, valuesToDerivations(v)...)
		}
	case starlark.Tuple:
		for _, v := range v {
			derivations = append(derivations, valuesToDerivations(v)...)
		}
	}
	return
}

func (b *Bramble) gc(_ []string) (err error) {
	if err = b.init(); err != nil {
		return
	}
	drvQueue, err := b.collectDerivationsToPreserve()
	if err != nil {
		return
	}
	pathsToKeep := map[string]struct{}{}

	// TODO: maybe this will get too big?
	drvCache := map[string]*Derivation{}

	loadDerivation := func(filename string) (drv *Derivation, err error) {
		if drv, ok := drvCache[filename]; ok {
			return drv, nil
		}
		drv, err = b.loadDerivation(filename)
		if err == nil {
			drvCache[filename] = drv
		}
		return drv, err
	}
	var do DerivationOutput
	var runtimeDep bool

	defer logger.Print(pathsToKeep)

	processedDerivations := map[DerivationOutput]bool{}
	for {
		if len(drvQueue) == 0 {
			break
		}
		// pop one off to process
		for do, runtimeDep = range drvQueue {
			break
		}
		delete(drvQueue, do)
		pathsToKeep[do.Filename] = struct{}{}
		toAdd, err := b.findDerivationsInputDerivationsToKeep(
			loadDerivation,
			pathsToKeep,
			do, runtimeDep)
		if err != nil {
			return err
		}
		for toAddID, toAddRuntimeDep := range toAdd {
			if thisIsRuntimeDep, ok := processedDerivations[toAddID]; !ok {
				// if we don't have it, add it
				drvQueue[toAddID] = toAddRuntimeDep
			} else if !thisIsRuntimeDep && toAddRuntimeDep {
				// if we do have it, but it's not a runtime dep, and this is, add it
				drvQueue[toAddID] = toAddRuntimeDep
			}
			// otherwise don't add it
		}
	}

	// delete everything in the store that's not in the map
	files, err := ioutil.ReadDir(b.store.StorePath)
	if err != nil {
		return
	}
	for _, file := range files {
		if _, ok := pathsToKeep[file.Name()]; !ok {
			logger.Print("deleting", file.Name())
			if err = os.RemoveAll(b.store.JoinStorePath(file.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

type derivationMap struct {
	Location    string
	Derivations map[string][]string
}

func (b *Bramble) collectDerivationsToPreserve() (drvQueue map[DerivationOutput]bool, err error) {
	registryFolder := b.store.JoinBramblePath("var", "config-registry")
	files, err := ioutil.ReadDir(registryFolder)
	if err != nil {
		return
	}

	drvQueue = map[DerivationOutput]bool{}
	for _, f := range files {
		var drvMap derivationMap
		registryLoc := filepath.Join(registryFolder, f.Name())
		f, err := os.Open(registryLoc)
		if err != nil {
			return nil, err
		}
		if _, err := toml.DecodeReader(f, &drvMap); err != nil {
			return nil, err
		}
		_ = f.Close()
		logger.Print("assembling derivations for", drvMap.Location)
		// delete the config if we can't find the project any more
		tomlLoc := filepath.Join(drvMap.Location, "bramble.toml")
		if !fileutil.PathExists(tomlLoc) {
			logger.Printfln("deleting cache for %q, it no longer exists", tomlLoc)
			if err := os.Remove(registryLoc); err != nil {
				return nil, err
			}
			continue
		}
		for _, list := range drvMap.Derivations {
			// TODO: check that these global entrypoints actually still exist
			for _, item := range list {
				parts := strings.Split(item, ":")
				drvQueue[DerivationOutput{
					Filename:   parts[0],
					OutputName: parts[1],
				}] = true
			}
		}
	}
	return
}

func (b *Bramble) findDerivationsInputDerivationsToKeep(
	loadDerivation func(string) (*Derivation, error),
	pathsToKeep map[string]struct{},
	do DerivationOutput, runtimeDep bool) (
	addToQueue map[DerivationOutput]bool, err error) {
	addToQueue = map[DerivationOutput]bool{}

	drv, err := loadDerivation(do.Filename)
	if err != nil {
		return
	}

	// keep all source paths for all derivations
	for _, p := range drv.SourcePaths {
		pathsToKeep[p] = struct{}{}
	}

	dependencyOutputs := map[string]bool{}
	if runtimeDep {
		for _, filename := range drv.Output(do.OutputName).Dependencies {
			// keep outputs for all runtime dependencies
			pathsToKeep[filename] = struct{}{}
			dependencyOutputs[filename] = false
		}
	}

	for _, inputDO := range drv.InputDerivations {
		idDrv, err := loadDerivation(inputDO.Filename)
		if err != nil {
			return nil, err
		}
		outPath := idDrv.Output(inputDO.OutputName).Path
		// found this derivation in an output, add it as a runtime dep
		if _, ok := dependencyOutputs[outPath]; ok {
			addToQueue[inputDO] = true
			dependencyOutputs[outPath] = true
		} else {
			addToQueue[inputDO] = false
		}
		// keep all derivations
		pathsToKeep[inputDO.Filename] = struct{}{}
	}
	for path, found := range dependencyOutputs {
		if !found {
			return nil, errors.Errorf(
				"derivation %s has output %s which was not "+
					"found as an output of any of its input derivations.",
				do.Filename, path)
		}
	}

	return nil, nil
}

func (b *Bramble) loadDerivation(filename string) (drv *Derivation, err error) {
	f, err := os.Open(b.store.JoinStorePath(filename))
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	drv = &Derivation{}
	return drv, json.NewDecoder(f).Decode(&drv)
}

func (b *Bramble) derivationBuild(args []string) error {
	return nil
}

func (b *Bramble) resolveModule(module string) (globals starlark.StringDict, err error) {
	if !strings.HasPrefix(module, b.config.Module.Name) {
		// TODO: support other modules
		debug.PrintStack()
		err = errors.Errorf("can't find module %s", module)
		return
	}

	if _, ok := b.moduleCache[module]; ok {
		filename, exists := b.filenameCache.LoadInverse(module)
		if !exists {
			return nil, errors.Errorf("module %q returns no matching filename", module)
		}
		return b.starlarkExecFile(module, filename)
	}

	path := module[len(b.config.Module.Name):]
	path = filepath.Join(b.configLocation, path)

	directoryWithNameExists := fileutil.PathExists(path)

	var directoryHasDefaultDotBramble bool
	if directoryWithNameExists {
		directoryHasDefaultDotBramble = fileutil.FileExists(path + "/default.bramble")
	}

	fileWithNameExists := fileutil.FileExists(path + BrambleExtension)

	switch {
	case directoryWithNameExists && directoryHasDefaultDotBramble:
		path += "/default.bramble"
	case fileWithNameExists:
		path += BrambleExtension
	default:
		err = errModuleDoesNotExist(module)
		return
	}

	return b.starlarkExecFile(module, path)
}

func (b *Bramble) moduleFromPath(path string) (module string, err error) {
	module = (b.config.Module.Name + "/" + b.relativePathFromConfig())
	if path == "" {
		return
	}

	// See if this path is actually the name of a module, for now we just
	// support one module.
	// TODO: search through all modules in scope for this config
	if strings.HasPrefix(path, b.config.Module.Name) {
		return path, nil
	}

	// if the relative path is nothing, we've already added the slash above
	if !strings.HasSuffix(module, "/") {
		module += "/"
	}

	// support things like bar/main.bramble:foo
	if strings.HasSuffix(path, BrambleExtension) && fileutil.FileExists(path) {
		return module + path[:len(path)-len(BrambleExtension)], nil
	}

	fullName := path + BrambleExtension
	if !fileutil.FileExists(fullName) {
		if !fileutil.FileExists(path + "/default.bramble") {
			return "", errors.Errorf("%q: no such file or directory", path)
		}
	}
	// we found it, return
	module += filepath.Join(path)
	return
}

func (b *Bramble) relativePathFromConfig() string {
	wd, _ := os.Getwd()
	relativePath, _ := filepath.Rel(b.configLocation, wd)
	if relativePath == "." {
		// don't add a dot to the path
		return ""
	}
	return relativePath
}

func (b *Bramble) parseModuleFuncArgument(args []string) (module, function string, err error) {
	if len(args) == 0 {
		logger.Print(`"bramble build" requires 1 argument`)
		return "", "", flag.ErrHelp
	}

	firstArgument := args[0]
	lastIndex := strings.LastIndex(firstArgument, ":")
	if lastIndex < 0 {
		logger.Print("module and function argument is not properly formatted")
		return "", "", flag.ErrHelp
	}
	path, function := firstArgument[:lastIndex], firstArgument[lastIndex+1:]
	module, err = b.moduleFromPath(path)
	return
}
