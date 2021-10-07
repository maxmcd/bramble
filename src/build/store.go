package build

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/maxmcd/bramble/pkg/chunkedarchive"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/sandbox"
	"github.com/maxmcd/bramble/src/logger"
	"github.com/pkg/errors"
)

var (
	PathPaddingCharacters = "bramble_store_padding"
	PathPaddingLength     = 50

	// BramblePrefixOfRecord is the prefix we use when hashing the build output
	// this allows us to get a consistent hash even if we're building in a
	// different location
	BramblePrefixOfRecord = "/home/bramble/bramble/bramble_store_padding/bramb" // TODO: could we make this more obviously fake?

	buildDirPrefix = "bramble_build_directory"
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
	Args   []string
	Mounts []string

	Stdin io.Reader
	Dir   string

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

		Network: false,
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

		// Cache for starlark file compilation.
		"var/star-cache",

		// Location to mount chroots for builds
		"var/builds",
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
	hshr := hasher.NewHasher()
	if _, err = hshr.Write([]byte(location)); err != nil {
		return
	}
	reg := s.joinBramblePath("var/config-registry")
	hash := hshr.String()
	configFileLocation := filepath.Join(reg, hash)
	return ioutil.WriteFile(configFileLocation, []byte(location), 0644)
}

func (s *Store) WriteBlob(src io.Reader) (hash string, err error) {
	h := hasher.NewHasher()
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
	storeDirectoryName += ("/" + PathPaddingCharacters[:extra])
	return storeDirectoryName, nil
}

func (s *Store) WriteDerivation(drv Derivation) (filename string, err error) {
	drv = formatDerivation(drv)
	filename = drv.Filename()
	fileLocation := s.joinStorePath(filename)
	return filename, ioutil.WriteFile(fileLocation, drv.json(), 0644)
}

func (s *Store) UploadDerivationsToCache(derivations []Derivation, cc *cacheClient) (err error) {
	bodyWriter := chunkedarchive.NewParallelBodyWriter(
		runtime.NumCPU(),
		func(rc io.ReadCloser) (out []string, err error) {
			buf := bufio.NewReader(rc)
			for {
				limited := io.LimitReader(buf, 4e6)
				hash, err := cc.postChunk(limited)
				if err != nil {
					return nil, err
				}
				out = append(out, hash)
				fmt.Println("Finished uploading chunk", hash)
				if _, err := buf.Peek(1); err != nil {
					break
				}
			}
			return out, rc.Close()
		},
	)

	// Loop through derivations
	for _, drv := range derivations {
		// Normalize them with the fixed prefix path
		normalized, err := s.normalizeDerivation(drv)
		if err != nil {
			return err
		}
		// Upload, could confirm hash
		if _, err := cc.postDerivation(normalized); err != nil {
			return err
		}
		// Loop through outputs and post them
		for _, output := range normalized.Outputs {
			// This will upload using the spawned queue in parallel
			toc, err := chunkedarchive.Archive(s.joinStorePath(output.Path), bodyWriter)
			if err != nil {
				return err
			}

			if _, err := cc.postOutout(toc); err != nil {
				return err
			}
		}
	}
	return
}
