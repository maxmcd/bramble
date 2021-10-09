package chunkedarchive

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/pkg/errors"
)

const (
	// footerSize is the number of bytes in the chunkedarchive footer.
	//
	// The footer is an empty gzip stream with no compression and an Extra
	// header of the form "%016xCHUNKA", where the 64 bit hex-encoded
	// number is the offset to the gzip stream of JSON TOC.
	//
	// 47 comes from:
	//
	//   10 byte gzip header +
	//   2 byte (LE16) length of extra, encoding 22 (16 hex digits + len("CHUNKA")) == "\x16\x00" +
	//   22 bytes of extra (fmt.Sprintf("%016xCHUNKA", tocGzipOffset))
	//   5 byte flate header
	//   8 byte gzip footer (two little endian uint32s: digest, size)
	footerSize = 47
	tocTarName = "chunkedarchive.index.json"
)

// StreamArchive and UnarchiveStream stick the chunked archive into a single
// file. So far this use case is just for local archiving and replacing bytes,
// but it implements some of the patterns present in crfs/stargz in case that is
// a future use-case for this format. For the time being we'll assume any
// network requests for files will download individual chunks from an index, but
// for now it seemed good to follow in this general direction over picking
// something arbitrary.
func StreamArchive(location string, output io.Writer) (err error) {
	buf := bufio.NewWriter(output)
	countW := &countWriter{w: buf}

	zw, _ := gzip.NewWriterLevel(countW, gzip.NoCompression)
	tw := tar.NewWriter(zw)
	bw := &tarBodyWriter{
		writer: tw,
		buf:    make([]byte, chunkSize),
	}
	toc, err := Archive(location, bw)
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	tocOff := countW.n
	// Write toc
	{
		zw, _ = gzip.NewWriterLevel(countW, gzip.NoCompression)
		tocJSON, err := json.MarshalIndent(toc, "", "  ")
		if err != nil {
			return err
		}
		tw := tar.NewWriter(zw)
		if err := tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     tocTarName,
			Size:     int64(len(tocJSON)),
		}); err != nil {
			return err
		}
		if _, err := tw.Write(tocJSON); err != nil {
			return err
		}

		if err := tw.Close(); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
	}

	// And a little footer with pointer to the TOC gzip stream.
	if _, err := countW.Write(footerBytes(tocOff)); err != nil {
		return err
	}
	return buf.Flush()
}

type tarBodyWriter struct {
	writer *tar.Writer
	buf    []byte
}

func (bw *tarBodyWriter) NewChunk(f io.ReadCloser) (func() ([]string, error), error) {
	out := []string{}
	for {
		h := hasher.New()
		tr := io.TeeReader(f, h)

		n, err := tr.Read(bw.buf)
		if err != nil {
			return nil, err
		}
		hash := h.String()
		out = append(out, hash)
		if err := bw.writer.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     hash,
			Size:     int64(n),
		}); err != nil {
			return nil, err
		}

		if _, err := bw.writer.Write(bw.buf[:n]); err != nil {
			return nil, err
		}
		if n != chunkSize {
			break
		}
	}
	return func() ([]string, error) {
		return out, nil
	}, nil
}

// footerBytes the 47 byte footer.
func footerBytes(tocOff int64) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, footerSize))
	gz, _ := gzip.NewWriterLevel(buf, gzip.NoCompression)
	gz.Header.Extra = []byte(fmt.Sprintf("%016xCHUNKA", tocOff))
	gz.Close()
	if buf.Len() != footerSize {
		panic(fmt.Sprintf("footer buffer = %d, not %d", buf.Len(), footerSize))
	}
	return buf.Bytes()
}

// countWriter counts how many bytes have been written to its wrapped
// io.Writer.
type countWriter struct {
	w io.Writer
	n int64
}

func (cw *countWriter) Write(p []byte) (n int, err error) {
	n, err = cw.w.Write(p)
	cw.n += int64(n)
	return
}

func StreamUnarchive(sr *io.SectionReader, location string) (err error) {
	// Pull TOC from footer
	toc := []TOCEntry{}
	{
		if sr.Size() < footerSize {
			return errors.Errorf("chunkedarchive size %d is smaller than the archive footer size", sr.Size())
		}
		var footer [footerSize]byte
		if _, err := sr.ReadAt(footer[:], sr.Size()-footerSize); err != nil {
			return errors.Errorf("error reading footer: %v", err)
		}
		tocOff, ok := parseFooter(footer[:])
		if !ok {
			return errors.Errorf("error parsing footer")
		}
		tocTargz := make([]byte, sr.Size()-tocOff-footerSize)
		if _, err := sr.ReadAt(tocTargz, tocOff); err != nil {
			return errors.Errorf("error reading %d byte TOC targz: %v", len(tocTargz), err)
		}
		zr, err := gzip.NewReader(bytes.NewReader(tocTargz))
		if err != nil {
			return errors.Errorf("malformed TOC gzip header: %v", err)
		}
		zr.Multistream(false)
		tr := tar.NewReader(zr)
		h, err := tr.Next()
		if err != nil {
			return errors.Errorf("failed to find tar header in TOC gzip stream: %v", err)
		}
		if h.Name != tocTarName {
			return errors.Errorf("TOC tar entry had name %q; expected %q", h.Name, tocTarName)
		}
		if err := json.NewDecoder(tr).Decode(&toc); err != nil {
			return errors.Errorf("error decoding TOC JSON: %v", err)
		}
	}
	// Seek to beginning of file and start writing files
	if _, err := sr.Seek(0, 0); err != nil {
		return err
	}
	gr, err := gzip.NewReader(sr)
	if err != nil {
		return err
	}
	return Unarchive(toc, &tarSerialHashFetcher{
		reader: tar.NewReader(gr),
		wg:     &sync.WaitGroup{},
	}, location)
}

type tarSerialHashFetcher struct {
	reader *tar.Reader
	wg     *sync.WaitGroup
}

var _ HashFetcher = new(tarSerialHashFetcher)

// wgLimitReader will call Done() on a waitgroup when reading is done
type wgLimitReader struct {
	reader io.Reader
	wg     *sync.WaitGroup
	done   bool
}

func (r *wgLimitReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	if err == io.EOF && !r.done {
		r.done = true
		r.wg.Done()
	}
	return n, err
}

func (hf *tarSerialHashFetcher) Lookup(hash string) (io.ReadCloser, error) {
	// Lookup will only read one chunk at a time and not proceed to read the
	// next header until the current reader has been completely read
	hf.wg.Wait()
	th, err := hf.reader.Next()
	if err != nil {
		return nil, err
	}
	if hash != th.Name {
		return nil, errors.New("tar header name and chunk hash do not match")
	}
	hf.wg.Add(1)
	return io.NopCloser(&wgLimitReader{
		reader: io.LimitReader(hf.reader, th.Size),
		wg:     hf.wg,
	}), nil
}

func parseFooter(p []byte) (tocOffset int64, ok bool) {
	if len(p) != footerSize {
		return 0, false
	}
	zr, err := gzip.NewReader(bytes.NewReader(p))
	if err != nil {
		return 0, false
	}
	extra := zr.Header.Extra
	if len(extra) != 16+len("CHUNKA") {
		return 0, false
	}
	if string(extra[16:]) != "CHUNKA" {
		return 0, false
	}
	tocOffset, err = strconv.ParseInt(string(extra[:16]), 16, 64)
	return tocOffset, err == nil
}
