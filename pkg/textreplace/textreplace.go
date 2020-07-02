package textreplace

import (
	"bytes"
	"io"
)

func ReplaceStrings(values [][]byte, old []byte, new []byte,
	source io.Reader, output io.Writer) (
	replacements int, matches map[string]struct{}, err error) {
	longestValueLength := 0
	for _, in := range values {
		if len(in) > longestValueLength {
			longestValueLength = len(in)
		}
	}
	matches = make(map[string]struct{})
	_, err = CopyWithFrames(output, source, nil, longestValueLength, func(b []byte) error {
		j := 0
		for {
		BEGIN:
			i := bytes.Index(b[j:], old)
			if i < 0 {
				break
			}
			j += i
			// Could use a trie here if the values list is very long.
			for _, input := range values {
				if len(input) > len(b[j:]) {
					continue
				}
				if bytes.Equal(b[j:j+len(input)], input) {
					matches[string(input)] = struct{}{}
					replacements++
					copy(b[j:j+len(old)], new)
					goto BEGIN
				}
			}
			break
		}
		return nil
	})
	return
}

func CopyWithFrames(dst io.Writer, src io.Reader,
	buf []byte, frameSize int, transform func(b []byte) error) (
	written int64, err error) {
	if buf == nil {
		size := 32 * 1024 // The default from io/io.go.
		buf = make([]byte, size)
	}
	firstPassOffset := frameSize
	for {
		nr, er := src.Read(buf[frameSize:])
		if nr > 0 {
			if err = transform(buf[firstPassOffset:]); err != nil {
				return
			}
			nw, ew := dst.Write(buf[firstPassOffset:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			// We write a subset of the read bytes on the first pass so
			// we
			if nr != nw && written != int64(nr-firstPassOffset) {
				err = io.ErrShortWrite
				break
			}
			if n := copy(buf[:frameSize], buf[nr:nr+frameSize]); n != frameSize {
				panic(frameSize)
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			nw, ew := dst.Write(buf[:frameSize])
			if ew != nil {
				err = ew
			}
			written += int64(nw)
			break
		}
		firstPassOffset = 0
	}
	return
}
