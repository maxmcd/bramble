package fileutil

import (
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

		// stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		// if !ok {
		// 	return fmt.Errorf("failed to get raw syscall.Stat_t data for '%s'", sourcePath)
		// }
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
	files, err = expandPathDirectories(files)
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
		// TODO: Commenting this out (and the one in the function above) because
		// we hit an issue where a file's group was `root`, but we can't write a
		// file as the root group. Seems fine to leave this out, files should be
		// greated with the default user and group of the of the current user,
		// but just commenting for now. Who knows what the future holds.
		//
		// stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		// if !ok {
		// 	return errors.Errorf("failed to get raw syscall.Stat_t data for '%s'", file)
		// }
		// if err := os.Lchown(destPath, int(stat.Uid), int(stat.Gid)); err != nil {
		//  return errors.WithStack(err)
		// }

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
func expandPathDirectories(files []string) (out []string, err error) {
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

// PathWithinDir checks if a path is within a given directory. It doesn't
// validate if the passed directory path is actually a directory. If the
// function returns a nil error the path is within the directory.
//
// Dir and path must be absolute paths
func PathWithinDir(dir, path string) (err error) {
	if !filepath.IsAbs(dir) {
		return errors.Errorf("directory %q is not an absolute path", dir)
	}
	if !filepath.IsAbs(path) {
		return errors.Errorf("path %q is not an absolute path", path)
	}
	relpath, err := filepath.Rel(dir, path)
	if err != nil {
		return err
	}
	if strings.Contains(relpath, "..") {
		return errors.Errorf("path %q is not within the %q directory", path, dir)
	}
	return nil
}

// Abs is similar to filepath.Abs but it allows you to pass a custom wd
func Abs(wd, path string) (string, error) {
	if !filepath.IsAbs(wd) {
		return "", errors.Errorf("working directory %q is not absolute", wd)
	}
	// TODO: windows
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Join(wd, path), nil
}
