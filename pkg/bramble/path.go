package bramble

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

var (

	// TODO: improve this error message
	ErrPathTooLong        = errors.New("calculated path is too long")
	ErrStoreDoesNotExist  = errors.New("calculated store path doesn't exist, did the location change?")
	PathPaddingCharacters = "bramble_store_padding"
	PaddingLength         = 512
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
	storeDirectoryName, err = calculatePaddedDirectoryName(bramblePath, 512)
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
		return "", ErrPathTooLong
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
