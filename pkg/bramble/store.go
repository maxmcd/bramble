package bramble

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

var (
	PathPaddingCharacters = "bramble_store_padding"
	PathPaddingLength     = 50

	ErrStoreDoesNotExist = errors.New("calculated store path doesn't exist, did the location change?")
)

func NewStore() (Store, error) {
	s := Store{}
	return s, s.ensureBramblePath()
}

type Store struct {
	bramblePath string
	storePath   string
}

func (s *Store) ensureBramblePath() (err error) {
	s.bramblePath = os.Getenv("BRAMBLE_PATH")
	if s.bramblePath == "" {
		var home string
		home, err = os.UserHomeDir()
		if err != nil {
			err = errors.Wrap(err, "error searching for users home directory")
			return
		}
		s.bramblePath = filepath.Join(home, "bramble")
	}
	if !filepath.IsAbs(s.bramblePath) {
		err = errors.Errorf("bramble path %s must be absolute", s.bramblePath)
		return
	}

	if _, err = os.Stat(s.bramblePath); err != nil {
		fmt.Println("bramble path directory doesn't exist, creating")
		if err = os.Mkdir(s.bramblePath, 0755); err != nil {
			return
		}
	}
	files, err := ioutil.ReadDir(s.bramblePath)
	if err != nil {
		err = errors.Wrap(err, "error listing files in the BRAMBLE_PATH")
		return
	}

	fileMap := map[string]struct{}{}
	for _, file := range files {
		fileMap[file.Name()] = struct{}{}
	}

	var storeDirectoryName string
	storeDirectoryName, err = calculatePaddedDirectoryName(s.bramblePath, PathPaddingLength)
	if err != nil {
		return
	}

	s.storePath = s.joinBramblePath(storeDirectoryName)

	// No files exist in the store, make the store
	if len(files) == 0 {
		if err = os.MkdirAll(s.storePath, 0755); err != nil {
			return
		}
		if err = os.Symlink("."+storeDirectoryName, s.joinBramblePath("store")); err != nil {
			return
		}
	}
	if _, ok := fileMap["tmp"]; !ok {
		// TODO: move this to a common cache directory or somewhere else that this would
		// be expected to be
		if err = os.Mkdir(s.joinBramblePath("tmp"), 0755); err != nil {
			return
		}
	}
	if _, ok := fileMap["var"]; !ok {
		if err = os.Mkdir(s.joinBramblePath("var"), 0755); err != nil {
			return
		}
		if err = os.Mkdir(s.joinBramblePath("var/config-registry"), 0755); err != nil {
			return
		}
		if err = os.Mkdir(s.joinBramblePath("var/star-cache"), 0755); err != nil {
			return
		}
	}

	// otherwise, check if the exact store path we need exists
	if _, err = os.Stat(s.storePath); err != nil {
		err = ErrStoreDoesNotExist
		return
	}

	return
}

func (s Store) joinStorePath(v ...string) string {
	return filepath.Join(append([]string{s.storePath}, v...)...)
}
func (s Store) joinBramblePath(v ...string) string {
	return filepath.Join(append([]string{s.bramblePath}, v...)...)
}

func (s Store) writeReader(src io.Reader, name string, validateHash string) (contentHash, path string, err error) {
	hasher := NewHasher()
	file, err := ioutil.TempFile(s.joinBramblePath("tmp"), "")
	if err != nil {
		err = errors.Wrap(err, "error creating a temporary file for a write to the store")
		return
	}
	tee := io.TeeReader(src, hasher)
	if _, err = io.Copy(file, tee); err != nil {
		err = errors.Wrap(err, "error writing to the temporary store file")
		return
	}
	fileName := hasher.String()
	if validateHash != "" && hasher.Sha256Hex() != validateHash {
		return hasher.Sha256Hex(), "", errHashMismatch
	}
	if name != "" {
		fileName += ("-" + name)
	}
	path = s.joinStorePath(fileName)
	if er := os.Rename(file.Name(), path); er != nil {
		return "", "", errors.Wrap(er, "error moving file into store")
	}
	return hasher.Sha256Hex(), path, nil
}

func (s Store) writeConfigLink(location string, derivations map[string][]string) (err error) {
	hasher := NewHasher()
	if _, err = hasher.Write([]byte(location)); err != nil {
		return
	}
	reg := s.joinBramblePath("var/config-registry")
	hash := hasher.String()
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
