package store

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/pkg/errors"
)

var (
	// does this ensure the same length always?
	BuildDirPattern       = "bramble_build_directory*"
	PathPaddingCharacters = "bramble_store_padding"
	PathPaddingLength     = 50

	ErrStoreDoesNotExist = errors.New("calculated store path doesn't exist, did the location change?")
)

func NewStore() (Store, error) {
	s := Store{}
	return s, ensureBramblePath(&s)
}

type Store struct {
	BramblePath string
	StorePath   string
}

func (s Store) IsEmpty() bool {
	return s.BramblePath == "" || s.StorePath == ""
}

func (s Store) TempDir() (tempDir string, err error) {
	return ioutil.TempDir(s.StorePath, BuildDirPattern)
}

func (s Store) TempBuildDir() (tempDir string, err error) {
	return ioutil.TempDir(filepath.Join(s.BramblePath, "var/builds"), "build-")
}

func ensureBramblePath(s *Store) (err error) {
	var exists bool
	// Prefer BRAMBLE_PATH if it's set. Otherwise use the folder "bramble" in
	// the user's home directory.
	s.BramblePath, exists = os.LookupEnv("BRAMBLE_PATH")
	if !exists {
		var home string
		home, err = os.UserHomeDir()
		if err != nil {
			return errors.Wrap(err, "error searching for users home directory")
		}
		s.BramblePath = filepath.Join(home, "bramble")
	} else {
		// Ensure we clean the path so that our padding calculation is consistent.
		s.BramblePath = filepath.Clean(s.BramblePath)
	}

	// No support for relative bramble paths.
	if !filepath.IsAbs(s.BramblePath) {
		return errors.Errorf("bramble path %s must be absolute", s.BramblePath)
	}

	if !fileutil.PathExists(s.BramblePath) {
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
		files, _ = ioutil.ReadDir(s.JoinBramblePath("var"))
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

	s.StorePath = s.JoinBramblePath(storeDirectoryName)

	// Add store folder with the correct padding and add a convenience symlink
	// in the bramble folder.
	if _, ok := fileMap["store"]; !ok {
		if err = os.MkdirAll(s.StorePath, 0755); err != nil {
			return err
		}
		if err = os.Symlink("."+storeDirectoryName, s.JoinBramblePath("store")); err != nil {
			return err
		}
	}

	folders := []string{
		// TODO: move this to a common cache directory or somewhere else that
		// this would be expected to be
		"tmp", // Tmp folder, probably shouldn't exist.

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
			if err = os.Mkdir(s.JoinBramblePath(folder), 0755); err != nil {
				return errors.Wrap(err, fmt.Sprintf("error creating bramble folder %q", folder))
			}
		}
	}

	// otherwise, check if the exact store path we need exists
	if _, err = os.Stat(s.StorePath); err != nil {
		return ErrStoreDoesNotExist
	}

	return
}

func (s Store) JoinStorePath(v ...string) string {
	return filepath.Join(append([]string{s.StorePath}, v...)...)
}
func (s Store) JoinBramblePath(v ...string) string {
	return filepath.Join(append([]string{s.BramblePath}, v...)...)
}

func (s Store) WriteReader(src io.Reader, name string, validateHash string) (contentHash, path string, err error) {
	hshr := hasher.NewHasher()
	file, err := ioutil.TempFile(s.JoinBramblePath("tmp"), "")
	if err != nil {
		err = errors.Wrap(err, "error creating a temporary file for a write to the store")
		return
	}
	tee := io.TeeReader(src, hshr)
	if _, err = io.Copy(file, tee); err != nil {
		err = errors.Wrap(err, "error writing to the temporary store file")
		return
	}
	fileName := hshr.String()
	if validateHash != "" && hshr.Sha256Hex() != validateHash {
		return hshr.Sha256Hex(), "", hasher.ErrHashMismatch
	}
	if name != "" {
		fileName += ("-" + name)
	}
	path = s.JoinStorePath(fileName)
	if err = file.Close(); err != nil {
		return "", "", err
	}
	if er := os.Rename(file.Name(), path); er != nil {
		return "", "", errors.Wrap(er, "error moving file into store")
	}
	if err = os.Chmod(path, 0444); err != nil {
		return
	}

	return hshr.Sha256Hex(), path, nil
}

func (s Store) WriteConfigLink(location string, derivations map[string][]string) (err error) {
	hshr := hasher.NewHasher()
	if _, err = hshr.Write([]byte(location)); err != nil {
		return
	}
	reg := s.JoinBramblePath("var/config-registry")
	hash := hshr.String()
	configFileLocation := filepath.Join(reg, hash+"-metadata.toml")

	f, err := os.OpenFile(configFileLocation,
		os.O_RDWR|os.O_APPEND|os.O_CREATE,
		0644)
	if err != nil {
		return
	}
	var dm derivationMap
	if _, err = toml.DecodeReader(f, &dm); err != nil {
		return
	}
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)

	dm.Location = location
	if dm.Derivations == nil {
		dm.Derivations = map[string][]string{}
	}
	for k, v := range derivations {
		dm.Derivations[k] = v
	}
	if err = toml.NewEncoder(f).Encode(dm); err != nil {
		return
	}
	return f.Close()
}

type derivationMap struct {
	Location    string
	Derivations map[string][]string
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
