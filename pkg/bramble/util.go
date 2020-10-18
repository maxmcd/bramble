package bramble

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"hash"
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

func commonFilepathPrefix(paths []string) string {
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

// Hasher is used to compute path hash values. Hasher implements io.Writer and
// takes a sha256 hash of the input bytes. The output string is a lowercase
// base32 representation of the first 160 bits of the hash
type Hasher struct {
	hash hash.Hash
}

func NewHasher() *Hasher {
	return &Hasher{
		hash: sha256.New(),
	}
}

func (h *Hasher) Write(b []byte) (n int, err error) {
	return h.hash.Write(b)
}

func (h *Hasher) String() string {
	return bytesToBase32Hash(h.hash.Sum(nil))
}
func (h *Hasher) Sha256Hex() string {
	return fmt.Sprintf("%x", h.hash.Sum(nil))
}

// bytesToBase32Hash copies nix here
// https://nixos.org/nixos/nix-pills/nix-store-paths.html
// Finally the comments tell us to compute the base32 representation of the
// first 160 bits (truncation) of a sha256 of the above string:
func bytesToBase32Hash(b []byte) string {
	var buf bytes.Buffer
	_, _ = base32.NewEncoder(base32.StdEncoding, &buf).Write(b[:20])
	return strings.ToLower(buf.String())
}

func hashFile(name string, file io.ReadCloser) (fileHash, filename string, err error) {
	defer file.Close()
	hasher := NewHasher()
	if _, err = hasher.Write([]byte(name)); err != nil {
		return
	}
	if _, err = io.Copy(hasher, file); err != nil {
		return
	}
	filename = fmt.Sprintf("%s-%s", hasher.String(), name)
	return
}

func cp(wd string, paths ...string) (err error) {
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
	if fileExists(dest) {
		return errors.New("copy destination can't be a file that exists")
	}

	toCopy := absPaths[:len(absPaths)-1]

	// "cp foo.txt bar.txt" or "cp ./foo ./bar" is a special case if it's just two
	// paths and they don't exist yet
	if len(toCopy) == 1 && !pathExists(dest) {
		f := toCopy[0]
		if isDir(f) {
			return errors.WithStack(copyDirectory(f, dest))
		}
		return errors.WithStack(copyFile(f, dest))
	}

	// otherwise copy each listed file into a directory with the given name
	for i, path := range toCopy {
		fi, err := os.Stat(path)
		if err != nil {
			return errors.Errorf("%q doesn't exist", paths[i])
		}
		if fi.IsDir() {
			destFolder := filepath.Join(dest, fi.Name())
			if err = createDirIfNotExists(destFolder, 0755); err != nil {
				return err
			}
			err = copyDirectory(path, filepath.Join(dest, fi.Name()))
		} else {
			err = copyFile(path, filepath.Join(dest, fi.Name()))
		}
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// copy directory will copy all of the contents of one directory into another directory
func copyDirectory(scrDir, dest string) error {
	entries, err := ioutil.ReadDir(scrDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(scrDir, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		fileInfo, err := os.Stat(sourcePath)
		if err != nil {
			return errors.WithStack(err)
		}

		stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("failed to get raw syscall.Stat_t data for '%s'", sourcePath)
		}

		switch fileInfo.Mode() & os.ModeType {
		case os.ModeDir:
			if err := createDirIfNotExists(destPath, 0755); err != nil {
				return errors.WithStack(err)
			}
			if err := copyDirectory(sourcePath, destPath); err != nil {
				return errors.WithStack(err)
			}
		case os.ModeSymlink:
			if err := copySymLink(sourcePath, destPath); err != nil {
				return errors.WithStack(err)
			}
		default:
			if err := copyFile(sourcePath, destPath); err != nil {
				return errors.WithStack(err)
			}
		}

		if err := os.Lchown(destPath, int(stat.Uid), int(stat.Gid)); err != nil {
			return errors.WithStack(err)
		}

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
// another directory, maintaining structure. Importantly it doesn't copy
// all the files in these directories, just the specific named paths.
func copyFilesByPath(prefix string, files []string, dest string) (err error) {
	files, err = expandPathDirectories(files)
	if err != nil {
		return err
	}

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
			if err := createDirIfNotExists(destPath, 0755); err != nil {
				return errors.WithStack(err)
			}
		case os.ModeSymlink:
			if err := copySymLink(file, destPath); err != nil {
				return errors.WithStack(err)
			}
		default:
			if err := copyFile(file, destPath); err != nil {
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

func copyFile(srcFile, dstFile string) error {
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

func createDirIfNotExists(dir string, perm os.FileMode) error {
	if pathExists(dir) {
		return nil
	}

	if err := os.MkdirAll(dir, perm); err != nil {
		return errors.Errorf("failed to create directory: '%s', error: '%s'", dir, err.Error())
	}

	return nil
}

func copySymLink(source, dest string) error {
	link, err := os.Readlink(source)
	if err != nil {
		return err
	}
	return os.Symlink(link, dest)
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !fi.IsDir()
}

func isDir(file string) bool {
	info, err := os.Stat(file)
	if err != nil {
		if err.(*os.PathError).Err != syscall.ENOENT {
			log.Fatalf("%s failed to access %s", err, file)
		}
		return false
	}
	return info.Mode().IsDir()
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func validSymlinkExists(path string) (dest string, ok bool) {
	if pathExists(path) {
		if link, err := os.Readlink(path); err != nil {
			return link, pathExists(link)
		}
	}
	return "", false
}

func lookPath(file string, path string) (string, error) {
	if strings.Contains(file, "/") {
		err := findExecutable(file)
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
		if err := findExecutable(path); err == nil {
			return path, nil
		}
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}
func findExecutable(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if m := d.Mode(); !m.IsDir() && m&0111 != 0 {
		return nil
	}
	return os.ErrPermission
}
