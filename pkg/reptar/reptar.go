package reptar

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

var zeroTime time.Time

// References:
// http://h2.jaguarpaw.co.uk/posts/reproducible-tar/
// https://reproducible-builds.org/docs/archives/

// Reptar creates a tar of a location. Reptar stands for reproducible tar and is
// intended to replicate the following gnu tar command:
//
//    tar - \
//    --sort=name \
//    --mtime="1970-01-01 00:00:00Z" \
//    --owner=0 --group=0 --numeric-owner \
//    --pax-option=exthdr.name=%d/PaxHeaders/%f,delete=atime,delete=ctime \
//    -cf
//
// This command is currently not complete and only works on very basic test
// cases. GNU Tar also adds padding to outputted files
func Archive(location string, out io.Writer) (err error) {
	// TODO: add our own null padding to match GNU Tar
	// TODO: test with hardlinks
	// TODO: confirm name sorting is identical in all cases
	// TODO: disallow absolute paths

	tw := tar.NewWriter(out)
	location = filepath.Clean(location)
	if err = filepath.Walk(location, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if location == path {
			return nil
		}
		var linkTarget string
		if isSymlink(fi) {
			var err error
			linkTarget, err = os.Readlink(path)
			if err != nil {
				return fmt.Errorf("%s: readlink: %w", fi.Name(), err)
			}
			// TODO: convert from absolute to relative
		}

		// GNU Tar adds a slash to the end of directories, but Go removes them
		if fi.IsDir() {
			path += "/"
		}
		hdr, err := tar.FileInfoHeader(fi, filepath.ToSlash(linkTarget))
		if err != nil {
			return err
		}

		// Setting an explicit unix epoch using time.Date(1970, time.January..)
		// resulted in zeros in the timestamp and not null, so we explicitly use
		// a null time
		hdr.ModTime = zeroTime
		hdr.AccessTime = zeroTime
		hdr.ChangeTime = zeroTime

		// It seems that both seeing these to 0 and using empty strings for
		// Gname and Uname is required
		hdr.Uid = 0
		hdr.Gid = 0
		hdr.Gname = ""
		hdr.Uname = ""

		// pax format
		hdr.Format = tar.FormatPAX

		hdr.Name = strings.TrimPrefix(path, location)

		if err = tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("%s: writing header: %w", hdr.Name, err)
		}

		if fi.IsDir() {
			return nil // directories have no contents
		}
		if hdr.Typeflag == tar.TypeReg {
			var file io.ReadCloser
			file, err = os.Open(path)
			if err != nil {
				return fmt.Errorf("%s: opening: %w", path, err)
			}
			_, err := io.Copy(tw, file)
			if err != nil {
				return fmt.Errorf("%s: copying contents: %v", fi.Name(), err)
			}
			_ = file.Close()
		}
		return nil
	}); err != nil {
		return
	}
	return tw.Close()
}

func isSymlink(fi os.FileInfo) bool {
	return fi.Mode()&os.ModeSymlink != 0
}

func Unarchive(in io.Reader, location string) error {
	reader := tar.NewReader(in)
	madeDir := map[string]bool{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "error reading next tar header")
		}
		rel := filepath.FromSlash(header.Name)
		abs := filepath.Join(location, rel)
		fi := header.FileInfo()
		mode := fi.Mode()
		switch mode & os.ModeType {
		case os.ModeDir:
			if err := os.MkdirAll(abs, 0755); err != nil {
				return err
			}
			madeDir[abs] = true
		case os.ModeSymlink:
			if err := os.Symlink(header.Linkname, abs); err != nil {
				return err
			}
		case os.ModeNamedPipe:
			if err := syscall.Mkfifo(abs, uint32(mode.Perm())); err != nil {
				return err
			}
		default:
			dir := filepath.Dir(abs)
			if !madeDir[dir] {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return err
				}
				madeDir[dir] = true
			}
			wf, err := os.OpenFile(abs, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode.Perm())
			if err != nil {
				return errors.WithStack(err)
			}
			var n int64
			if n, err = io.Copy(wf, reader); err != nil {
				return errors.WithStack(err)
			}
			if err := wf.Close(); err != nil {
				return fmt.Errorf("error writing to %s: %v", abs, err)
			}
			if n != header.Size {
				return fmt.Errorf("only wrote %d bytes to %s; expected %d", n, abs, header.Size)
			}
		}
	}

}
