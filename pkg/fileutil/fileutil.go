package fileutil

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"

	"github.com/pkg/errors"
)

func CommonFilepathPrefix(paths []string) string {
	sep := byte(os.PathSeparator)
	if len(paths) == 0 {
		return string(sep)
	}

	c := []byte(path.Clean(paths[0]))
	c = append(c, sep)

	for _, v := range paths[1:] {
		v = path.Clean(v) + string(sep)
		if len(v) < len(c) {
			c = c[:len(v)]
		}
		for i := 0; i < len(c); i++ {
			if v[i] != c[i] {
				c = c[:i]
				break
			}
		}
	}

	for i := len(c) - 1; i >= 0; i-- {
		if c[i] == sep {
			c = c[:i+1]
			break
		}
	}

	return string(c)
}

// LS is a silly debugging function, don't use it
func LS(wd string) {
	entries, err := os.ReadDir(wd)
	if err != nil {
		fmt.Println(err)
	}
	for _, entry := range entries {
		fi, _ := entry.Info()
		fmt.Printf("%s %d %s %s\n", fi.Mode(), fi.Size(), fi.ModTime(), entry.Name())
	}
	if err != nil {
		panic(err)
	}
}

func CP(wd string, paths ...string) (err error) {
	if len(paths) == 1 {
		return errors.New("copy takes at least two arguments")
	}
	absPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			absPaths = append(absPaths, filepath.Join(wd, path))
		} else {
			absPaths = append(absPaths, path)
		}
	}
	dest := absPaths[len(paths)-1]
	// if dest exists and it's not a directory
	if FileExists(dest) {
		return errors.New("copy destination can't be a file that exists")
	}

	toCopy := absPaths[:len(absPaths)-1]

	// "cp foo.txt bar.txt" or "cp ./foo ./bar" is a special case if it's just
	// two paths and they don't exist yet
	if len(toCopy) == 1 && !PathExists(dest) {
		f := toCopy[0]
		if IsDir(f) {
			return errors.WithStack(CopyDirectory(f, dest))
		}
		return errors.WithStack(CopyFile(f, dest))
	}

	// otherwise copy each listed file into a directory with the given name
	for i, path := range toCopy {
		// TODO: this should be Lstat
		fi, err := os.Stat(path)
		if err != nil {
			return errors.Errorf("%q doesn't exist", paths[i])
		}
		if fi.IsDir() {
			destFolder := filepath.Join(dest, fi.Name())
			if err = CreateDirIfNotExists(destFolder, 0755); err != nil {
				return err
			}
			err = CopyDirectory(path, filepath.Join(dest, fi.Name()))
		} else {
			err = CopyFile(path, filepath.Join(dest, fi.Name()))
		}
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func ReplaceAll(filepath, old, new string) (err error) {
	f, err := os.Stat(filepath)
	if err != nil {
		return err
	}
	input, err := ioutil.ReadFile(filepath)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(
		filepath,
		[]byte(strings.ReplaceAll(string(input), old, new)),
		f.Mode(),
	)
}

// copy directory will copy all of the contents of one directory into another
// directory
func CopyDirectory(scrDir, dest string) error {
	entries, err := ioutil.ReadDir(scrDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(scrDir, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		fileInfo, err := os.Lstat(sourcePath)
		if err != nil {
			return errors.WithStack(err)
		}

		stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		_ = stat
		if !ok {
			return fmt.Errorf("failed to get raw syscall.Stat_t data for '%s'", sourcePath)
		}
		switch fileInfo.Mode() & os.ModeType {
		case os.ModeSymlink:
			if err := CopySymLink(sourcePath, destPath); err != nil {
				return errors.WithStack(err)
			}
		case os.ModeDir:
			if err := CreateDirIfNotExists(destPath, 0755); err != nil {
				return errors.WithStack(err)
			}
			if err := CopyDirectory(sourcePath, destPath); err != nil {
				return errors.WithStack(err)
			}
		default:
			if err := CopyFile(sourcePath, destPath); err != nil {
				return errors.WithStack(err)
			}
		}

		// if err := os.Lchown(destPath, int(stat.Uid), int(stat.Gid)); err != nil {
		// 	return errors.WithStack(err)
		// }

		isSymlink := entry.Mode()&os.ModeSymlink != 0
		if !isSymlink {
			if err := os.Chmod(destPath, entry.Mode()); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}

// TODO: combine the duplicate logic in these two

// CopyFiles takes a list of absolute paths to files and copies them into
// another directory, maintaining structure. Importantly it doesn't copy all the
// files in these directories, just the specific named paths.
func CopyFilesByPath(prefix string, files []string, dest string) (err error) {
	files, err = ExpandPathDirectories(files)
	if err != nil {
		return err
	}

	sort.Slice(files, func(i, j int) bool { return len(files[i]) < len(files[j]) })
	for _, file := range files {
		destPath := filepath.Join(dest, strings.TrimPrefix(file, prefix))
		fileInfo, err := os.Lstat(file)
		if err != nil {
			return errors.Wrap(err, "error finding source file")
		}

		stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		if !ok {
			return errors.Errorf("failed to get raw syscall.Stat_t data for '%s'", file)
		}

		switch fileInfo.Mode() & os.ModeType {
		case os.ModeDir:
			if err := CreateDirIfNotExists(destPath, 0755); err != nil {
				return errors.WithStack(err)
			}
		case os.ModeSymlink:
			if err := CopySymLink(file, destPath); err != nil {
				return errors.WithStack(err)
			}
		default:
			if err := CopyFile(file, destPath); err != nil {
				return errors.WithStack(err)
			}
		}

		if err := os.Lchown(destPath, int(stat.Uid), int(stat.Gid)); err != nil {
			return errors.WithStack(err)
		}

		// TODO: when does this happen???
		isSymlink := fileInfo.Mode()&os.ModeSymlink != 0
		if !isSymlink {
			if err := os.Chmod(destPath, fileInfo.Mode()); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return
}

// takes a list of paths and adds all files in all subdirectories
func ExpandPathDirectories(files []string) (out []string, err error) {
	for _, file := range files {
		if err = filepath.Walk(file,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				out = append(out, path)
				return nil
			}); err != nil {
			return
		}
	}
	return
}

func CopyFile(srcFile, dstFile string) error {
	in, err := os.Open(srcFile)
	if err != nil {
		return errors.WithStack(err)
	}
	defer in.Close()

	fi, err := in.Stat()
	if err != nil {
		return errors.WithStack(err)
	}
	out, err := os.OpenFile(dstFile, os.O_CREATE|os.O_RDWR, fi.Mode())
	if err != nil {
		return errors.WithStack(err)
	}

	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func CreateDirIfNotExists(dir string, perm os.FileMode) error {
	if PathExists(dir) {
		return nil
	}

	if err := os.MkdirAll(dir, perm); err != nil {
		return errors.Errorf("failed to create directory: '%s', error: '%s'", dir, err.Error())
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

// FileExists will only return true if the path is a file, not a directory
func FileExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !fi.IsDir()
}

// DirExists will only return true if the path exists and is a directory
func DirExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

func IsDir(file string) bool {
	info, err := os.Stat(file)
	if err != nil {
		if err.(*os.PathError).Err != syscall.ENOENT {
			log.Fatalf("%s failed to access %s", err, file)
		}
		return false
	}
	return info.Mode().IsDir()
}

func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func ValidSymlinkExists(path string) (dest string, ok bool) {
	if PathExists(path) {
		if link, err := os.Readlink(path); err != nil {
			return link, PathExists(link)
		}
	}
	return "", false
}

func LookPath(file string, path string) (string, error) {
	if strings.Contains(file, "/") {
		err := FindExecutable(file)
		if err == nil {
			return file, nil
		}
		return "", &exec.Error{Name: file, Err: err}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			// Unix shell semantics: path element "" means "."
			dir = "."
		}
		path := filepath.Join(dir, file)
		if err := FindExecutable(path); err == nil {
			return path, nil
		}
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}
func FindExecutable(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if m := d.Mode(); !m.IsDir() && m&0111 != 0 {
		return nil
	}
	return os.ErrPermission
}

// TestTmpDir is intended to be used in tests and will remove itself when the
// test run is over
func TestTmpDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "bramble-test-")
	if err != nil {
		panic(err)
	}
	if t != nil {
		t.Cleanup(func() {
			os.RemoveAll(dir)
		})
	}
	return dir
}
