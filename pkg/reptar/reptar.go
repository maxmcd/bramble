package reptar

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var zeroTime time.Time

// References:
// http://h2.jaguarpaw.co.uk/posts/reproducible-tar/
// https://reproducible-builds.org/docs/archives/

// Reptar creates a tar of a location. Reptar stands for reproducible tar and
// is intended to replicate the following gnu tar command:
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
func Reptar(location string, out io.Writer) (err error) {
	// TODO: add our own null padding to match GNU Tar
	// TODO: test with hardlinks
	// TODO: confirm name sorting is identical in all cases
	// TODO: disallow absolute paths

	tw := tar.NewWriter(out)
	if err = filepath.Walk(location, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
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
		// resulted in zeros in the timestamp and not null, so we explicitly
		// use a null time
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
		}
		return nil
	}); err != nil {
		return
	}
	return tw.Flush()
}

// GzipReptar just wraps reptar in gzip. This seems like a good place for a
// godzilla pun but I couldn't think of anything. Contributions welcome
func GzipReptar(location string, out io.Writer) (err error) {
	w := gzip.NewWriter(out)
	defer w.Close()
	return Reptar(location, w)
}

func isSymlink(fi os.FileInfo) bool {
	return fi.Mode()&os.ModeSymlink != 0
}
