package chunkedarchive

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

const (
	chunkSize int = 4e6
)

// TOCEntry is an entry in the file index's TOC (Table of Contents).
type TOCEntry struct {
	// Name is the tar entry's name. It is the complete path
	// stored in the tar file, not just the base name.
	Name string `json:"name"`

	// Type is one of "dir", "reg", "symlink", "hardlink", "char",
	// "block", "fifo"
	Type string `json:"type"`

	// Size, for regular files, is the logical size of the file.
	Size int64 `json:"size,omitempty"`

	// LinkName, for symlinks and hardlinks, is the link target.
	LinkName string `json:"linkName,omitempty"`

	// Mode is the permission and mode bits.
	Mode int64 `json:"mode,omitempty"`

	// DevMajor is the major device number for "char" and "block" types.
	DevMajor int `json:"devMajor,omitempty"`

	// DevMinor is the major device number for "char" and "block" types.
	DevMinor int `json:"devMinor,omitempty"`

	// NumLink is the number of entry names pointing to this entry.
	// Zero means one name references this entry.
	NumLink int

	// Xattrs are the extended attribute for the entry.
	Xattrs map[string][]byte `json:"xattrs,omitempty"`

	// Body references hashes of body content
	Body []string `json:"digest,omitempty"`
}

type fileInfo struct{ e *TOCEntry }

var _ os.FileInfo = fileInfo{}

func (fi fileInfo) Name() string       { return path.Base(fi.e.Name) }
func (fi fileInfo) IsDir() bool        { return fi.e.Type == "dir" }
func (fi fileInfo) Size() int64        { return fi.e.Size }
func (fi fileInfo) ModTime() time.Time { return time.Time{} }
func (fi fileInfo) Sys() interface{}   { return fi.e }
func (fi fileInfo) Mode() (m os.FileMode) {
	m = os.FileMode(fi.e.Mode) & os.ModePerm
	switch fi.e.Type {
	case "dir":
		m |= os.ModeDir
	case "symlink":
		m |= os.ModeSymlink
	case "char":
		m |= os.ModeDevice | os.ModeCharDevice
	case "block":
		m |= os.ModeDevice
	case "fifo":
		m |= os.ModeNamedPipe
	}
	// TODO: ModeSetuid, ModeSetgid, if/as needed.
	return m
}

type BodyWriter interface {
	NewChunk(io.ReadCloser) (func() ([]string, error), error)
}

func Archive(location string, bw BodyWriter) (toc []TOCEntry, err error) {
	type entryPromise struct {
		entry   TOCEntry
		promise func() ([]string, error)
	}
	queue := []entryPromise{}

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

		// TODO: Could likely remove the tar dependency here pretty easily
		hdr, err := tar.FileInfoHeader(fi, filepath.ToSlash(linkTarget))
		if err != nil {
			return errors.WithStack(err)
		}

		var xattrs map[string][]byte
		if hdr.Xattrs != nil {
			xattrs = make(map[string][]byte)
			for k, v := range hdr.Xattrs {
				xattrs[k] = []byte(v)
			}
		}

		ent := TOCEntry{
			Name:   strings.TrimPrefix(path, location),
			Size:   fi.Size(),
			Mode:   hdr.Mode,
			Xattrs: xattrs,
		}

		switch hdr.Typeflag {
		case tar.TypeLink:
			// TODO: will this ever happen? File will just be read as a file?
			ent.Type = "hardlink"
			ent.LinkName = hdr.Linkname
		case tar.TypeSymlink:
			ent.Type = "symlink"
			ent.LinkName = hdr.Linkname
		case tar.TypeDir:
			ent.Type = "dir"
		case tar.TypeReg:
			ent.Type = "reg"
			ent.Size = hdr.Size
		case tar.TypeFifo:
			ent.Type = "fifo"
		default:
			return fmt.Errorf("unsupported input tar entry %q", hdr.Typeflag)
		}

		if fi.IsDir() || hdr.Typeflag != tar.TypeReg || fi.Size() == 0 {
			queue = append(queue, entryPromise{entry: ent})
			return nil // directories have no contents
		}
		var file io.ReadCloser
		file, err = os.Open(path)
		if err != nil {
			return fmt.Errorf("%s: opening: %w", path, err)
		}
		promise, err := bw.NewChunk(file)
		if err != nil {
			return fmt.Errorf("%s: reading: %w", path, err)
		}
		queue = append(queue, entryPromise{entry: ent, promise: promise})
		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}
	for _, ep := range queue {
		if ep.promise != nil {
			ep.entry.Body, err = ep.promise()
			if err != nil {
				return nil, err
			}
		}
		toc = append(toc, ep.entry)
	}
	return
}

type ParallelBodyWriter struct {
	sem chan struct{}
	cb  func(io.ReadCloser) ([]string, error)
}

var _ BodyWriter = new(ParallelBodyWriter)

func NewParallelBodyWriter(numParallel int, cb func(io.ReadCloser) ([]string, error)) *ParallelBodyWriter {
	bw := &ParallelBodyWriter{
		sem: make(chan struct{}, numParallel),
		cb:  cb,
	}
	return bw
}

func (bw *ParallelBodyWriter) NewChunk(body io.ReadCloser) (func() ([]string, error), error) {
	type result struct {
		hashes []string
		err    error
	}
	// Take slot to do work
	bw.sem <- struct{}{}
	out := make(chan result)
	go func() {
		r := result{}
		r.hashes, r.err = bw.cb(body)
		out <- r
	}()
	return func() ([]string, error) {
		r := <-out
		// Free up slot
		<-bw.sem
		return r.hashes, r.err
	}, nil
}

type HashFetcher interface {
	Lookup(hash string) (io.ReadCloser, error)
}

func Unarchive(toc []TOCEntry, fetcher HashFetcher, location string) (err error) {
	errChan := make(chan error)
	type Chunk struct {
		hash string
		body io.ReadCloser
	}
	fetchchan := make(chan Chunk, 20) // arbitrary, just to not block
	go func() {
		for _, ent := range toc {
			for _, hash := range ent.Body {
				body, err := fetcher.Lookup(hash)
				if err != nil {
					errChan <- err
				}
				fetchchan <- Chunk{body: body, hash: hash}
			}
		}
	}()

	madeDir := map[string]bool{}
	for _, ent := range toc {
		rel := filepath.FromSlash(ent.Name)
		abs := filepath.Join(location, rel)
		fi := fileInfo{&ent}
		mode := fi.Mode()
		switch mode & os.ModeType {
		case os.ModeDir:
			if err := os.MkdirAll(abs, 0755); err != nil {
				return err
			}
			madeDir[abs] = true
		case os.ModeSymlink:
			if err := os.Symlink(ent.LinkName, abs); err != nil {
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
			for _, hash := range ent.Body {
				var chunk Chunk
				select {
				case err := <-errChan:
					return err
				case chunk = <-fetchchan:
				}
				if chunk.hash != hash {
					panic("toc hashes must match")
				}
				i, err := io.Copy(wf, chunk.body)
				if closeErr := chunk.body.Close(); closeErr != nil && err == nil {
					err = closeErr
				}
				n += i
				if err != nil {
					return errors.Wrapf(err, "error writing to %s", abs)
				}
			}
			if err := wf.Close(); err != nil {
				return fmt.Errorf("error writing to %s: %v", abs, err)
			}
			if n != ent.Size {
				return fmt.Errorf("only wrote %d bytes to %s; expected %d", n, abs, ent.Size)
			}
		}
	}

	return
}

func isSymlink(fi os.FileInfo) bool {
	return fi.Mode()&os.ModeSymlink != 0
}

func makedev(major, minor int64) int {
	return int(((major & 0xfff) << 8) | (minor & 0xff) | ((major &^ 0xfff) << 32) | ((minor & 0xfffff00) << 12))
}
