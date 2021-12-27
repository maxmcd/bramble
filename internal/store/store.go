package store

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/maxmcd/bramble/internal/logger"
	"github.com/maxmcd/bramble/internal/tracing"
	"github.com/maxmcd/bramble/pkg/chunkedarchive"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/sandbox"
	"github.com/pkg/errors"
	"github.com/rhnvrm/simples3"
	"go.opentelemetry.io/otel/trace"
)

var (
	PathPaddingCharacters = "bramble_store_padding"
	PathPaddingLength     = 50

	// BramblePrefixOfRecord is the prefix we use when hashing the build output
	// this allows us to get a consistent hash even if we're building in a
	// different location
	BramblePrefixOfRecord = "/home/bramble/bramble/bramble_store_padding/bramb" // TODO: could we make this more obviously fake?

	buildDirPrefix = "bramble_build_directory"

	tracer = tracing.Tracer("build")
)

func NewStore(bramblePath string) (*Store, error) {
	s := &Store{derivationCache: newDerivationsMap()}
	return s, ensureBramblePath(s, bramblePath)
}

type Store struct {
	BramblePath string
	StorePath   string

	derivationCache *derivationsMap
}

func (s *Store) checkForBuiltDerivationOutputs(drv Derivation) (outputs []Output, built bool, err error) {
	filename := drv.Filename()
	existingDrv, exists, err := s.LoadDerivation(filename)
	if err != nil {
		return
	}
	// It's not built if it doesn't exist
	if !exists {
		return nil, false, nil
	}
	// It's not built if it doesn't have the outputs we need
	return existingDrv.Outputs, !existingDrv.missingOutput(), err
}

type RunDerivationOptions struct {
	Args    []string
	Network bool
	Stdin   io.Reader
	Dir     string

	Mounts        []string
	HiddenPaths   []string
	ReadOnlyPaths []string
}

func (s *Store) RunDerivation(ctx context.Context, drv Derivation, opts RunDerivationOptions) (err error) {
	copy, _ := drv.copyWithOutputValuesReplaced()

	PATH := copy.Env["PATH"]
	if PATH != "" {
		PATH = ":" + PATH
	}
	PATH = s.joinStorePath(drv.output(drv.mainOutput()).Path, "/bin") + PATH
	copy.Env["PATH"] = PATH
	sbx := sandbox.Sandbox{
		Mounts: append([]string{s.StorePath + ":ro"}, opts.Mounts...),
		Env:    copy.env(),
		Args:   opts.Args,
		Stdin:  opts.Stdin,
		Stderr: os.Stderr,
		Stdout: os.Stdout,
		Dir:    opts.Dir,

		HiddenPaths:   opts.HiddenPaths,
		ReadOnlyPaths: opts.ReadOnlyPaths,

		Network: opts.Network,
	}
	return sbx.Run(ctx)
}

func (s *Store) LoadDerivation(filename string) (drv Derivation, found bool, err error) {
	defer logger.Debug("loadDerivation ", filename, " ", drv)
	drv, found = s.derivationCache.Load(filename)
	if found && !drv.missingOutput() {
		// if it has outputs return now
		return drv, found, nil
	}
	loc := s.joinStorePath(filename)
	if !fileutil.FileExists(loc) {
		// If we have the derivation in memory just return it
		if found {
			return drv, true, nil
		}
		// Doesn't exist
		return drv, false, nil
	}
	f, err := os.Open(loc)
	if err != nil {
		return drv, false, errors.WithStack(err)
	}
	defer func() { _ = f.Close() }()
	drv = s.newDerivation()
	if err = json.NewDecoder(f).Decode(&drv); err != nil {
		return
	}
	s.derivationCache.Store(drv)
	return drv, true, nil
}

func ensureBramblePath(s *Store, bramblePath string) (err error) {
	if p, ok := os.LookupEnv("BRAMBLE_PATH"); ok {
		bramblePath = p
	}
	if bramblePath == "" {
		var home string
		home, err = os.UserHomeDir()
		if err != nil {
			return errors.Wrap(err, "error searching for users home directory")
		}
		s.BramblePath = filepath.Join(home, "bramble")
	} else {
		// Ensure we clean the path so that our padding calculation is consistent.
		s.BramblePath = filepath.Clean(bramblePath)
	}

	// No support for relative bramble paths.
	if !filepath.IsAbs(s.BramblePath) {
		return errors.Errorf("bramble path %s must be absolute", s.BramblePath)
	}

	if !fileutil.PathExists(s.BramblePath) {
		// TODO: use logger
		fmt.Println("bramble path directory doesn't exist, creating")
		if err = os.Mkdir(s.BramblePath, 0755); err != nil {
			return err
		}
	}

	fileMap := map[string]struct{}{}
	{
		// List all files in the bramble folder.
		files, err := ioutil.ReadDir(s.BramblePath)
		if err != nil {
			return errors.Wrap(err, "error listing files in the BRAMBLE_PATH")
		}
		for _, file := range files {
			fileMap[file.Name()] = struct{}{}
		}

		// Specifically check for files in the var folder.
		files, _ = ioutil.ReadDir(s.joinBramblePath("var"))
		if len(files) > 0 {
			for _, file := range files {
				fileMap["var/"+file.Name()] = struct{}{}
			}
		}
	}

	var storeDirectoryName string
	if storeDirectoryName, err = calculatePaddedDirectoryName(s.BramblePath, PathPaddingLength); err != nil {
		return err
	}

	s.StorePath = s.joinBramblePath(storeDirectoryName)

	// Add store folder with the correct padding and add a convenience symlink
	// in the bramble folder.
	if _, ok := fileMap["store"]; !ok {
		if err = os.MkdirAll(s.StorePath, 0755); err != nil {
			return err
		}
		if err = os.Symlink("."+storeDirectoryName, s.joinBramblePath("store")); err != nil {
			return err
		}
	}

	folders := []string{
		"var", // The var folder.

		// Metadata for config files to store recently built derivations so that
		// they're not wiped during GC
		"var/config-registry",

		// Dependency metadata
		"var/dependencies",
	}

	for _, folder := range folders {
		if _, ok := fileMap[folder]; !ok {
			if err = os.Mkdir(s.joinBramblePath(folder), 0755); err != nil {
				return errors.Wrap(err, fmt.Sprintf("error creating bramble folder %q", folder))
			}
		}
	}

	// otherwise, check if the exact store path we need exists
	if !fileutil.PathExists(s.StorePath) {
		return errors.New("calculated store path doesn't exist, did the location change?")
	}

	return
}

func (s *Store) joinStorePath(v ...string) string {
	return filepath.Join(append([]string{s.StorePath}, v...)...)
}

func (s *Store) joinBramblePath(v ...string) string {
	return filepath.Join(append([]string{s.BramblePath}, v...)...)
}

func (s *Store) outputFoldersExist(outputs []Output) (exists bool, err error) {
	for _, output := range outputs {
		fi, err := os.Stat(s.joinStorePath(output.Path))
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		if !fi.IsDir() {
			return false, nil
		}
	}
	return true, nil
}

func (s *Store) WriteConfigLink(location string) (err error) {
	hshr := hasher.New()
	if _, err = hshr.Write([]byte(location)); err != nil {
		return
	}
	reg := s.joinBramblePath("var/config-registry")
	hash := hshr.String()
	configFileLocation := filepath.Join(reg, hash)
	return ioutil.WriteFile(configFileLocation, []byte(location), 0644)
}

func (s *Store) WriteBlob(src io.Reader) (hash string, err error) {
	h := hasher.New()
	tee := io.TeeReader(src, h)
	f, err := os.CreateTemp("", "")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, tee); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	hash = h.String()
	return hash, os.Rename(f.Name(), s.joinStorePath(hash))
}

func calculatePaddedDirectoryName(bramblePath string, paddingLength int) (storeDirectoryName string, err error) {
	paddingLen := paddingLength -
		len(bramblePath) - // parent folder lengths
		1 - // slash before directory
		1 // slash after the directory

	if paddingLen <= 0 {
		return "", errors.Errorf(
			"Bramble location creates a path that is too long. "+
				"Location '%s' is too long to create a directory less than %d in length",
			bramblePath, paddingLen)
	}

	paddingSize := len(PathPaddingCharacters)
	repetitions := paddingLen / (paddingSize + 1)
	extra := paddingLen % (paddingSize + 1)
	for i := 0; i < repetitions; i++ {
		storeDirectoryName += ("/" + PathPaddingCharacters)
	}
	if extra == 0 {
		// Of we have 0 extra characters the path ends up being too short.
		// If we have this path and we add one character to the bramble path
		// /bb/bramble_store_padding/bramble_store_padding/b/
		// We get this, which is one too short:
		// /bbb/bramble_store_padding/bramble_store_padding/
		// So we pad this path like so:
		// /bbb/bramble_store_padding/bramble_store_paddingb/
		// TODO: find a more elegant way to handle this
		storeDirectoryName += PathPaddingCharacters[0:1]
	} else {
		storeDirectoryName += ("/" + PathPaddingCharacters[:extra])
	}

	return storeDirectoryName, nil
}

func (s *Store) WriteDerivation(drv Derivation) (filename string, err error) {
	drv = formatDerivation(drv)
	filename = drv.Filename()
	fileLocation := s.joinStorePath(filename)
	return filename, ioutil.WriteFile(fileLocation, drv.JSON(), 0644)
}

type CacheClient interface {
	PostChunk(context.Context, string, io.Reader) (string, bool, error)
	PostDerivation(context.Context, Derivation) (string, bool, error)
	PostOutput(context.Context, OutputRequestBody) (bool, error)
	OutputExists(ctx context.Context, name string) (bool, error)
}

func (s *Store) UploadDerivationsToCache(ctx context.Context, derivations []Derivation, cc CacheClient) (err error) {
	var span trace.Span
	ctx, span = tracer.Start(ctx, "store.UploadDerivationsToCache")
	defer span.End()
	var cancel func()
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	bodyWriter := chunkedarchive.NewParallelBodyWriter(
		12,
		func(ctx context.Context, name string, rc io.ReadCloser) (out []string, err error) {
			select {
			case <-ctx.Done():
				return nil, context.Canceled
			default:
			}
			buf := bufio.NewReader(rc)
			for {
				// TODO: hash the body before uploading to confirm it doesn't already exist
				limited := io.LimitReader(buf, 4e6)
				hash, _, err := cc.PostChunk(ctx, name, limited)
				if err != nil {
					return nil, err
				}
				out = append(out, hash)
				if _, err := buf.Peek(1); err != nil {
					break
				}
			}
			return out, rc.Close()
		},
	)

	uploaded := map[string]struct{}{}
	errChan := make(chan error)
	doneChan := make(chan struct{})
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup

	// Loop through derivations
	for _, drv := range derivations {
		// Normalize them with the fixed prefix path
		normalized, err := s.normalizeDerivation(drv)
		if err != nil {
			return err
		}
		// Upload, could confirm hash
		if _, _, err := cc.PostDerivation(ctx, normalized); err != nil {
			return err
		}
		// Loop through outputs and post them
		for _, output := range normalized.Outputs {
			if _, ok := uploaded[output.Path]; ok {
				continue
			}
			uploaded[output.Path] = struct{}{}

			exists, err := cc.OutputExists(ctx, output.Path)
			if err != nil {
				return err
			}
			if exists {
				fmt.Printf("output %s for derivation %s already exists\n", output.Path, drv.Filename())
				continue
			} else {
				fmt.Printf("output %s for derivation %s doesn't exist\n", output.Path, drv.Filename())
			}

			wg.Add(1)
			go func(output Output) {
				// Limit parallelism
				sem <- struct{}{}
				defer func() { <-sem }()
				// This will upload using the spawned queue in parallel
				toc, err := chunkedarchive.Archive(ctx, bodyWriter, s.joinStorePath(output.Path))
				if err != nil {
					errChan <- err
					return
				}

				if _, err := cc.PostOutput(ctx, OutputRequestBody{
					TOC:    toc,
					Output: output,
				}); err != nil {
					errChan <- err
				}
				wg.Done()
			}(output)
		}
	}

	go func() {
		wg.Wait()
		doneChan <- struct{}{}
	}()
	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-errChan:
		return err
	case <-doneChan:
		return nil
	}
}

type S3CacheClient struct {
	s3 *simples3.S3
}

type fakeSizeSeeker struct {
	buf *bytes.Buffer
	loc int64
}

func (fss fakeSizeSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		fss.loc = offset
		// Ignore seeks beyond loc
		return fss.loc, nil
	case io.SeekCurrent:
		return 0, nil
	case io.SeekEnd:
		fss.loc -= offset
		// Ignore seeks beyond loc
		return int64(fss.buf.Len()) - fss.loc, nil
	}
	panic("unimplemented")
}
func (fss fakeSizeSeeker) Read(p []byte) (n int, err error) { return fss.buf.Read(p) }

var _ CacheClient = new(S3CacheClient)

func NewS3CacheClient(s3 *simples3.S3) CacheClient {
	return &S3CacheClient{s3: s3}
}

func fileUpload(s3 *simples3.S3, body *bytes.Buffer, ui simples3.UploadInput, info string) (bool, error) {
	checkIt := "https://store.bramble.run/" + ui.ObjectKey
	fmt.Println("Uploading", info)
	resp, err := http.Head(checkIt)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
	case http.StatusOK:
		return true, nil
	default:
		var buf bytes.Buffer
		if resp.Body != nil {
			_, _ = io.Copy(&buf, resp.Body)
		}
		return false, errors.Errorf("unexpected response code %d for url %q: %s", resp.StatusCode, checkIt, buf.String())
	}
	// TODO: Could reduce memory overhead here
	buf := &bytes.Buffer{}
	w := gzip.NewWriter(buf)
	if _, err := io.Copy(w, bytes.NewBuffer(body.Bytes())); err != nil {
		return false, err
	}
	if err := w.Close(); err != nil {
		return false, err
	}

	ui.Body = fakeSizeSeeker{buf: body}
	if _, err := s3.FileUpload(ui); err != nil {
		return false, err
	}

	ui.Body = fakeSizeSeeker{buf: buf}
	ui.FileName += ".gz"
	ui.ObjectKey += ".gz"
	if _, err := s3.FileUpload(ui); err != nil {
		return false, err
	}
	return false, nil
}

func (cc *S3CacheClient) PostChunk(ctx context.Context, name string, r io.Reader) (string, bool, error) {
	var buf bytes.Buffer
	h := hasher.New()
	tee := io.TeeReader(r, h)
	if _, err := io.Copy(&buf, tee); err != nil {
		return "", false, err
	}
	exists, err := fileUpload(cc.s3, &buf, simples3.UploadInput{
		Bucket:      "bramble",
		ACL:         "public-read",
		ObjectKey:   "chunk/" + h.String(),
		FileName:    h.String(),
		ContentType: "application/octet-stream",
	}, fmt.Sprintf("chunk %s for %s", h.String(), name))
	return "", exists, err
}
func (cc *S3CacheClient) PostDerivation(ctx context.Context, drv Derivation) (string, bool, error) {
	filename := drv.Filename()

	exists, err := fileUpload(cc.s3, bytes.NewBuffer([]byte(drv.JSON())), simples3.UploadInput{
		Bucket:      "bramble",
		ACL:         "public-read",
		ObjectKey:   "derivation/" + drv.Filename(),
		FileName:    drv.Filename(),
		ContentType: "application/json",
	}, "derivation "+filename)
	return filename, exists, err
}
func (cc *S3CacheClient) PostOutput(ctx context.Context, req OutputRequestBody) (bool, error) {
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(req); err != nil {
		return false, err
	}
	exists, err := fileUpload(cc.s3, buf, simples3.UploadInput{
		Bucket:      "bramble",
		ACL:         "public-read",
		ObjectKey:   "output/" + req.Output.Path,
		FileName:    req.Output.Path,
		ContentType: "application/json",
	}, "output "+req.Output.Path)
	return exists, err
}

func (cc *S3CacheClient) OutputExists(ctx context.Context, name string) (bool, error) {
	// url := "https://store.bramble.run/output/" + name
	url := "https://bramble.nyc3.digitaloceanspaces.com/output/" + name
	resp, err := http.Head(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, errors.Errorf("unexpected response code when requesting %s: %d", url, resp.StatusCode)
	}
}
