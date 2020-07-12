package bramble

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/pkg/errors"
)

// CopyFiles takes a list of absolute paths to files and copies them into
// another directory, maintaining structure
func CopyFiles(files []string, dest string) error {
	prefix := CommonPrefix(files)
	sort.Slice(files, func(i, j int) bool { return len(files[i]) < len(files[j]) })
	for _, file := range files {
		destPath := filepath.Join(dest, strings.TrimPrefix(file, prefix))

		fileInfo, err := os.Stat(file)
		if err != nil {
			return errors.Wrap(err, "error finding source file")
		}

		stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		if !ok {
			return errors.Errorf("failed to get raw syscall.Stat_t data for '%s'", file)
		}

		switch fileInfo.Mode() & os.ModeType {
		case os.ModeDir:
			if err := CreateIfNotExists(destPath, 0755); err != nil {
				return err
			}
		case os.ModeSymlink:
			if err := CopySymLink(file, destPath); err != nil {
				return err
			}
		default:
			if err := Copy(file, destPath); err != nil {
				return err
			}
		}

		if err := os.Lchown(destPath, int(stat.Uid), int(stat.Gid)); err != nil {
			return err
		}

		// TODO: when does this happen???
		isSymlink := fileInfo.Mode()&os.ModeSymlink != 0
		if !isSymlink {
			if err := os.Chmod(destPath, fileInfo.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

func Copy(srcFile, dstFile string) error {
	out, err := os.Create(dstFile)
	if err != nil {
		return err
	}

	defer out.Close()

	in, err := os.Open(srcFile)
	defer in.Close()
	if err != nil {
		return err
	}

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return nil
}

func Exists(filePath string) bool {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}

	return true
}

func CreateIfNotExists(dir string, perm os.FileMode) error {
	if Exists(dir) {
		return nil
	}

	if err := os.MkdirAll(dir, perm); err != nil {
		return fmt.Errorf("failed to create directory: '%s', error: '%s'", dir, err.Error())
	}

	return nil
}

func CopySymLink(source, dest string) error {
	link, err := os.Readlink(source)
	if err != nil {
		return err
	}
	return os.Symlink(link, dest)
}
