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
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"runtime/trace"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
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
	"google.golang.org/protobuf/proto"

	"github.com/maxmcd/bramble/pkg/assert"
	"github.com/maxmcd/bramble/pkg/bramblepb"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/maxmcd/bramble/pkg/store"
	"github.com/maxmcd/bramble/pkg/textreplace"
)

func init() {
	// It's easier to start giving away free coffee than it is to take away
	// free coffee

	// I think this would allow storing arbitrary state in function closures
	// and make the codebase much harder to reason about. Maybe we want this
	// level of complexity at some point, but nice to avoid for now
	resolve.AllowLambda = false
	resolve.AllowNestedDef = false

	// recursion is probably ok, open to allowing it
	resolve.AllowRecursion = false

	// sets seem harmless tho?
	resolve.AllowSet = true
	// see little need for this (currently), but open to allowing it
	resolve.AllowFloat = false
}

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

	afterDerivation bool

	moduleEntrypoint string
	calledFunction   string

	derivations *DerivationsMap

	credentials *syscall.Credential
}

// AfterDerivation is called when a builtin function is called that can't be
// run before an derivation call. This allows us to track when
func (b *Bramble) AfterDerivation() { b.afterDerivation = true }
func (b *Bramble) CalledDerivation(filename string) error {
	if b.afterDerivation && !b.derivations.Has(filename) {
		return errors.New("build context is dirty, can't call derivation after cmd() or other builtins")
	}
	return nil
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
	fmt.Println("Building derivation", filename)
	if err = b.buildDerivation(ctx, drv); err != nil {
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
	return
}

func (b *Bramble) buildDerivation(ctx context.Context, drv *Derivation) (err error) {
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
	switch drv.Builder {
	case "fetch_url":
		err = b.fetchURLBuilder(ctx, drvCopy, outputPaths)
	case "fetch_git":
		err = b.fetchGitBuilder(ctx, drvCopy, outputPaths)
	case "function":
		err = b.dockerFunctionBuilder(ctx, drvCopy, buildDir, outputPaths)
	default:
		err = b.dockerRegularBuilder(ctx, drvCopy, buildDir, outputPaths)
	}
	if err != nil {
		return
	}

	if drv.Builder == "fetch_url" {
		// fetch url just hashes the directory and moves it into the output
		// location, no archiving and byte replacing
		return b.hashAndMoveFetchURL(ctx, drv, outputPaths["out"])
	}
	return b.hashAndMoveBuildOutputs(ctx, drv, outputPaths)
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
			return err
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
		fmt.Println("Output at ", newPath)
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
	case *starlark.Function:
		meta := functionBuilderMeta{
			ModuleCache: map[string]string{},
		}
		globals := v.Globals()
		var match bool
		for name, fn := range globals {
			if fn == v {
				match = true
				meta.Function = name
				break
			}
		}
		if !match {
			return errors.Errorf("function %q must be a global function to be used in a function builder", v)
		}
		filename := v.Position().Filename()
		var fileExists bool
		meta.Module, fileExists = b.filenameCache.Load(filename)
		if !fileExists {
			return errors.Errorf("filename %q not found in filenameCache", filename)
		}

		var moduleExists bool
		meta.ModuleCache[meta.Module], moduleExists = b.moduleCache[meta.Module]
		if !moduleExists {
			return errors.Errorf("module %q not found in moduleCache", meta.Module)
		}

		set, err := b.importGraph.Descendents(meta.Module)
		if err != nil {
			return err
		}
		for _, m := range set {
			moduleName := m.(string)
			meta.ModuleCache[moduleName] = b.moduleCache[moduleName]
		}

		drv.Builder = "function"

		b, _ := json.Marshal(meta)
		drv.Env["function_builder_meta"] = string(b)
		for _, p := range meta.ModuleCache {
			drv.SourcePaths = append(drv.SourcePaths, p)
		}
		// TODO: put these validation checks somehere singular?
		sort.Strings(drv.SourcePaths)
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
	path, err := b.DownloadFile(ctx, url, hash)
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
	path, err := b.DownloadFile(ctx, url, hash)
	if err != nil {
		return err
	}
	// TODO: what if this package changes?
	if err = archiver.Unarchive(path, outputPaths["out"]); err != nil {
		return errors.Wrap(err, "error unpacking url archive")
	}
	return nil
}

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

func (b *Bramble) regularBuilder(drv *Derivation, buildDir string, outputPaths map[string]string) (err error) {
	builderLocation := drv.Builder

	if _, err := os.Stat(builderLocation); err != nil {
		return errors.Wrap(err, "error checking if builder location exists")
	}
	cmd := exec.Command(builderLocation, drv.Args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: b.credentials,
	}
	cmd.Dir = filepath.Join(buildDir, drv.BuildContextRelativePath)
	cmd.Env = drv.env()
	for outputName, outputPath := range outputPaths {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", outputName, outputPath))
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (b *Bramble) constructBrambleProto() *bramblepb.Bramble {
	derivations := map[string]*bramblepb.Derivation{}
	b.derivations.Range(func(filename string, drv *Derivation) bool {
		derivations[filename] = drv.constructDerivationProto()
		return true
	})

	store := &bramblepb.Store{
		BramblePath: b.store.BramblePath,
		StorePath:   b.store.StorePath,
	}
	return &bramblepb.Bramble{
		Derivations: derivations,
		Store:       store,
		// TODO: issue that this is a pointer?
		FilenameCache: b.filenameCache.forward,
	}
}

func brambleFunctionBuildSingleton() (err error) {
	stdinBytes, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	var functionBuild bramblepb.FunctionBuild
	if err := proto.Unmarshal(stdinBytes, &functionBuild); err != nil {
		return errors.Wrap(err, "error unmarshalling FunctionBuild proto")
	}

	b := &Bramble{
		store: store.Store{
			StorePath:   functionBuild.Bramble.Store.StorePath,
			BramblePath: functionBuild.Bramble.Store.BramblePath,
		},

		moduleEntrypoint: functionBuild.FunctionBuilderMeta.Module,
		calledFunction:   functionBuild.FunctionBuilderMeta.Function,
		moduleCache:      functionBuild.FunctionBuilderMeta.ModuleCache,
		filenameCache:    NewBiStringMap(),
		importGraph:      NewAcyclicGraph(),
		derivations:      &DerivationsMap{},
	}
	session, err := b.newSession(
		filepath.Join(functionBuild.BuildDirectory, functionBuild.Derivation.BuildContextRelativePath),
		functionBuild.Derivation.Env,
	)

	if err != nil {
		return err
	}

	for k, v := range functionBuild.Bramble.FilenameCache {
		b.filenameCache.Store(k, v)
	}

	for name, drvProto := range functionBuild.Bramble.Derivations {
		b.derivations.Set(name, constructDerivationFromProto(drvProto))
	}

	b.thread = &starlark.Thread{Load: b.load}
	if err = b.initPredeclared(); err != nil {
		return
	}
	globals, err := b.resolveModule(functionBuild.FunctionBuilderMeta.Module)
	if err != nil {
		return
	}

	d := starlark.NewDict(len(functionBuild.OutputPaths))
	for name, path := range functionBuild.OutputPaths {
		_ = d.SetKey(starlark.String(name), starlark.String(path))
		session.env[name] = path
	}
	_, err = starlark.Call(
		b.thread,
		globals[functionBuild.FunctionBuilderMeta.Function].(*starlark.Function),
		starlark.Tuple{session, d}, nil,
	)
	return err
}

func (b *Bramble) dockerFunctionBuilder(ctx context.Context, drv *Derivation, buildDir string, outputPaths map[string]string) (err error) {
	region := trace.StartRegion(ctx, "dockerFunctionBuilder")
	defer region.End()
	var meta functionBuilderMeta
	if err = json.Unmarshal([]byte(drv.Env["function_builder_meta"]), &meta); err != nil {
		return errors.Wrap(err, "error parsing function_builder_meta")
	}

	brambleProto := b.constructBrambleProto()
	functionBuild := &bramblepb.FunctionBuild{
		Bramble:             brambleProto,
		FunctionBuilderMeta: meta.constructFunctionBuilderMetaProto(),
		OutputPaths:         outputPaths,
		BuildDirectory:      buildDir,
		Derivation:          drv.constructDerivationProto(),
	}
	buf, err := proto.Marshal(functionBuild)
	if err != nil {
		return err
	}
	builderReader := bytes.NewReader(buf)

	return b.runDockerBuild(ctx,
		drv.filename(), runDockerBuildOptions{
			buildDir:           buildDir,
			outputPaths:        outputPaths,
			stdin:              builderReader,
			cmd:                []string{"/bin/bramble", BrambleFunctionBuildHiddenCommand},
			mountBrambleBinary: true,
		})
}

func (b *Bramble) functionBuilder(drv *Derivation, buildDir string, outputPaths map[string]string) (err error) {
	var meta functionBuilderMeta
	if err = json.Unmarshal([]byte(drv.Env["function_builder_meta"]), &meta); err != nil {
		return errors.Wrap(err, "error parsing function_builder_meta")
	}
	session, err := b.newSession(buildDir, drv.Env)
	if err != nil {
		return
	}
	if session.currentDirectory, err = session.cd(filepath.Join(buildDir, drv.BuildContextRelativePath)); err != nil {
		return
	}
	for outputName, outputPath := range outputPaths {
		session.setEnv(outputName, outputPath)
	}
	return b.callInlineDerivationFunction(
		outputPaths,
		meta,
		session,
	)
}

// DownloadFile downloads a file into the store. Must include an expected hash
// of the downloaded file as a hex string of a  sha256 hash
func (b *Bramble) DownloadFile(ctx context.Context, url string, hash string) (path string, err error) {
	fmt.Printf("Downloading url %s\n", url)

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

func (b *Bramble) replaceOutputValuesInCmd(cmd *Cmd) (err error) {
	outputs := cmd.searchForDerivationOutputs()
	replacer, err := b.stringsReplacerForOutputs(outputs)
	if err != nil {
		return
	}
	cmd.Path = replacer.Replace(cmd.Path)
	for i, arg := range cmd.Args {
		cmd.Args[i] = replacer.Replace(arg)
	}
	for i, env := range cmd.Env {
		cmd.Env[i] = replacer.Replace(env)
	}
	return nil
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

func (b *Bramble) buildDerivationOutputs(dos DerivationOutputs) (err error) {
	graph := b.assembleDerivationDependencyGraph(dos)

	var wg sync.WaitGroup
	errChan := make(chan error)
	semaphore := make(chan struct{}, 2)

	if err = graph.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		graph.Walk(func(v dag.Vertex) tfdiags.Diagnostics {
			semaphore <- struct{}{}
			defer func() {
				<-semaphore
			}()
			// serial for now

			if root, ok := v.(string); ok && root == "root" {
				return nil
			}
			do := v.(DerivationOutput)
			drv := b.derivations.Get(do.Filename)
			if drv.Output(do.OutputName).Path != "" {
				return nil
			}
			wg.Add(1)
			if err := b.buildDerivationIfNew(ctx, drv); err != nil {
				// Passing the error might block, so we need an explicit Done call here.
				wg.Done()
				errChan <- err
				return nil
			}
			wg.Done()
			return nil
		})
		errChan <- nil
	}()
	err = <-errChan
	cancel() // Call cancel on the context, no-op if nothing is running
	if err != nil {
		// If we receive an error cancel the context and wait for any jobs that are running.
		wg.Wait()
	}
	return err
}

func (b *Bramble) callInlineDerivationFunction(
	outputPaths map[string]string, meta functionBuilderMeta, session Session) (err error) {
	// TODO: investigate if you actually need to create a new bramble
	newBramble := &Bramble{
		// retain from parent
		config:         b.config,
		configLocation: b.configLocation,
		store:          b.store,

		// populate for this task
		moduleEntrypoint: meta.Module,
		calledFunction:   meta.Function,
		moduleCache:      meta.ModuleCache,
		filenameCache:    NewBiStringMap(),
		importGraph:      NewAcyclicGraph(),
	}
	newBramble.thread = &starlark.Thread{Load: newBramble.load}
	// this will pass the session to cmd and os
	if err = newBramble.initPredeclared(); err != nil {
		return
	}
	newBramble.derivations = b.derivations

	globals, err := newBramble.resolveModule(meta.Module)
	if err != nil {
		return
	}

	d := starlark.NewDict(len(outputPaths))
	for name, path := range outputPaths {
		_ = d.SetKey(starlark.String(name), starlark.String(path))
	}
	_, err = starlark.Call(
		newBramble.thread,
		globals[meta.Function].(*starlark.Function),
		starlark.Tuple{session, d}, nil,
	)
	return
}

func (b *Bramble) reset() {
	b.afterDerivation = false
	b.moduleCache = map[string]string{}
	b.filenameCache = NewBiStringMap()
}

func (b *Bramble) init() (err error) {
	if b.configLocation != "" {
		return errors.New("can't initialize Bramble twice")
	}

	if err = b.parseCredentials(); err != nil {
		return err
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

func (b *Bramble) parseCredentials() (err error) {
	setUIDString, uidExists := os.LookupEnv("BRAMBLE_SET_UID")
	setGIDString, gidExists := os.LookupEnv("BRAMBLE_SET_GID")
	if !uidExists || !gidExists {
		return nil
	}
	uid, err := strconv.Atoi(setUIDString)
	if err != nil {
		return errors.Wrap(err, "converting BRAMBLE_SET_UID to int")
	}
	gid, err := strconv.Atoi(setGIDString)
	if err != nil {
		return errors.Wrap(err, "converting BRAMBLE_SET_GID to int")
	}
	b.credentials = &syscall.Credential{
		Gid: uint32(gid),
		Uid: uint32(uid),
	}
	return nil
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
	debugger := starlark.NewBuiltin("debugger", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		// TODO: https://github.com/google/starlark-go/issues/304
		repl.REPL(thread, b.predeclared)
		return nil, nil
	})

	b.predeclared = starlark.StringDict{
		"derivation": b.derivationFn,
		"os":         NewOS(b),
		"assert":     assertGlobals["assert"],
		"debugger":   debugger,
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

var BrambleExtension string = ".bramble"

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

// testErrorReporter reports errors during an individual test
type testErrorReporter struct {
	errors []error
}

func (e *testErrorReporter) Error(err error) {
	e.errors = append(e.errors, err)
}
func (e *testErrorReporter) FailNow() bool { return false }

func (b *Bramble) compilePath(path string) (prog *starlark.Program, err error) {
	compiledProgram, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "error opening moduleCache storeLocation")
	}
	return starlark.CompiledProgram(compiledProgram)
}

func (b *Bramble) sourceProgram(moduleName, filename string) (prog *starlark.Program, err error) {
	b.filenameCache.Store(filename, moduleName)
	storeLocation, ok := b.moduleCache[moduleName]
	if ok {
		// we have a cached binary location in the cache map, so we just use that
		return b.compilePath(b.store.JoinStorePath(storeLocation))
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
		return b.compilePath(relStoreLocation)
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

func (b *Bramble) ExecFile(moduleName, filename string) (globals starlark.StringDict, err error) {
	prog, err := b.sourceProgram(moduleName, filename)
	if err != nil {
		return
	}
	g, err := prog.Init(b.thread, b.predeclared)
	g.Freeze()
	return g, err
}

func (b *Bramble) test(args []string) (err error) {
	failFast := true
	if err = b.init(); err != nil {
		return
	}
	location := "."
	if len(args) > 0 {
		location = args[0]
	}
	testFiles, err := findBrambleFiles(location)
	if err != nil {
		return errors.Wrap(err, "error finding test files")
	}

	for _, filename := range testFiles {
		moduleName, err := b.moduleNameFromFileName(filename)
		if err != nil {
			return err
		}
		b.reset()
		absFilename, _ := filepath.Abs(filename)
		fmt.Printf("Running tests in file %q\n", filename)
		globals, err := b.ExecFile(moduleName, absFilename)
		if err != nil {
			return err
		}
		for name, fn := range globals {
			if !strings.HasPrefix(name, "test_") {
				continue
			}
			starFn, ok := fn.(*starlark.Function)
			if !ok {
				continue
			}
			fmt.Printf("\trunning test %q\n", name)
			errors := testErrorReporter{}
			assert.SetReporter(b.thread, &errors)
			_, err = starlark.Call(b.thread, starFn, nil, nil)
			if len(errors.errors) > 0 {
				fmt.Printf("\nGot %d errors while running %q in %q:\n", len(errors.errors), name, filename)
				for _, err := range errors.errors {
					fmt.Print(starutil.AnnotateError(err))
				}
				if failFast {
					return errQuiet
				}
			}
			if err != nil {
				return err
			}
		}
	}
	return
}

func (b *Bramble) repl(_ []string) (err error) {
	if err := b.init(); err != nil {
		return err
	}
	repl.REPL(b.thread, b.predeclared)
	return nil
}

func (b *Bramble) run(args []string) (err error) {
	if len(args) == 0 {
		err = errHelp
		return
	}
	set := flag.NewFlagSet("bramble", flag.ExitOnError)
	var all bool
	// TODO
	set.BoolVar(&all, "all", false, "run all public functions recursively")
	if err = set.Parse(args); err != nil {
		return
	}

	if err = b.init(); err != nil {
		return
	}

	{
		f, err := os.Create(filepath.Join(b.configLocation, "trace.out"))
		if err != nil {
			return err
		}
		if err = trace.Start(f); err != nil {
			return err
		}
		defer trace.Stop()
	}

	assert.SetReporter(b.thread, runErrorReporter{})

	module, fn, err := b.argsToImport(args)
	if err != nil {
		return
	}
	globals, err := b.resolveModule(module)
	if err != nil {
		return
	}
	b.calledFunction = fn
	b.moduleEntrypoint = module
	toCall, ok := globals[fn]
	if !ok {
		return errors.Errorf("global function %q not found", fn)
	}

	values, err := starlark.Call(&starlark.Thread{}, toCall, nil, nil)
	if err != nil {
		return errors.Wrap(err, "error running")
	}
	returnedDerivations := valuesToDerivations(values)

	if err = b.buildDerivationOutputs(b.derivationsToDerivationOutputs(returnedDerivations)); err != nil {
		return
	}

	return b.writeConfigMetadata(returnedDerivations)
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

	defer fmt.Println(pathsToKeep)

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
			fmt.Println("deleting", file.Name())
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
		fmt.Println("assembling derivations for", drvMap.Location)
		// delete the config if we can't find the project any more
		tomlLoc := filepath.Join(drvMap.Location, "bramble.toml")
		if !fileutil.PathExists(tomlLoc) {
			fmt.Printf("deleting cache for %q, it no longer exists\n", tomlLoc)
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
		return b.ExecFile(module, filename)
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

	return b.ExecFile(module, path)
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
			return "", errors.Errorf("can't find module at %q", path)
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

func (b *Bramble) argsToImport(args []string) (module, function string, err error) {
	if len(args) == 0 {
		return "", "", errRequiredFunctionArgument
	}

	firstArgument := args[0]
	if !strings.Contains(firstArgument, ":") {
		function = firstArgument
		module = b.config.Module.Name
		if loc := b.relativePathFromConfig(); loc != "" {
			module += ("/" + loc)
		}
	} else {
		parts := strings.Split(firstArgument, ":")
		if len(parts) != 2 {
			return "", "", errors.New("function name has too many colons")
		}
		var path string
		path, function = parts[0], parts[1]
		module, err = b.moduleFromPath(path)
	}

	return
}
