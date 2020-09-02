package bramble

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

var (
	PathPaddingCharacters = "bramble_store_padding"
	PathPaddingLength     = 50

	ErrStoreDoesNotExist = errors.New("calculated store path doesn't exist, did the location change?")
)

func ensureBramblePath() (bramblePath, storePath string, err error) {
	bramblePath = os.Getenv("BRAMBLE_PATH")
	if bramblePath == "" {
		var home string
		home, err = os.UserHomeDir()
		if err != nil {
			err = errors.Wrap(err, "error searching for users home directory")
			return
		}
		bramblePath = filepath.Join(home, "bramble")
	}
	if !filepath.IsAbs(bramblePath) {
		err = errors.Errorf("bramble path %s must be absolute", bramblePath)
		return
	}

	if _, err = os.Stat(bramblePath); err != nil {
		fmt.Println("bramble path directory doesn't exist, creating")
		if err = os.Mkdir(bramblePath, 0755); err != nil {
			return
		}
	}

	files, err := ioutil.ReadDir(bramblePath)
	if err != nil {
		err = errors.Wrap(err, "error listing files in the BRAMBLE_PATH")
		return
	}

	var storeDirectoryName string
	storeDirectoryName, err = calculatePaddedDirectoryName(bramblePath, PathPaddingLength)
	if err != nil {
		return
	}

	storePath = filepath.Join(bramblePath, storeDirectoryName)

	// No files exist in the store, make the store
	if len(files) == 0 {
		if err = os.MkdirAll(storePath, 0755); err != nil {
			return
		}
		if err = os.Symlink("."+storeDirectoryName, filepath.Join(bramblePath, "store")); err != nil {
			return
		}
		// TODO: move this to a common cache directory or somewhere else that this would
		// be expected to be
		if err = os.Mkdir(filepath.Join(bramblePath, "tmp"), 0755); err != nil {
			return
		}
	}

	// otherwise, check if the exact store path we need exists
	if _, err = os.Stat(storePath); err != nil {
		err = ErrStoreDoesNotExist
		return
	}

	return
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
