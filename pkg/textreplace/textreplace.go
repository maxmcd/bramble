package textreplace

import (
	"bytes"
	"errors"
	"io"
	"strings"
)

var (
	ErrNotSameLength = errors.New("old and new prefixes must be the same length")
)

// ReplaceStringsPrefix replaces the prefix of matching strings in a byte
// stream. All values to be replaced must have the same prefix and that prefix
// must be the same length as the new value.
func ReplaceStringsPrefix(source io.Reader, output io.Writer, values []string, old string, new string) (
	replacements int, matches map[string]struct{}, err error) {
	if len(old) != len(new) {
		return 0, nil, ErrNotSameLength
	}
	longestValueLength := 0
	for _, in := range values {
		if len(in) > longestValueLength {
			longestValueLength = len(in)
		}
	}

	matches = make(map[string]struct{})
	_, err = CopyWithFrames(source, output, nil, longestValueLength, func(b []byte) error {
		j := 0
		for {
		BEGIN:
			i := bytes.Index(b[j:], []byte(old))
			if i < 0 {
				break
			}
			j += i
			// Could use a trie here if the values list is very long.
			for _, input := range values {
				// if the input is longer than the remaining buffer length continue
				if len(input) > len(b[j:]) {
					continue
				}
				if string(b[j:j+len(input)]) == input {
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

// replaceStringsPrefixReplacer uses the strings.Replacer to replace the values,
// currently just used as a benchmark comparison
func replaceStringsPrefixReplacer(source io.Reader, output io.Writer, values []string, old string, new string) (
	err error) {
	if len(old) != len(new) {
		return ErrNotSameLength
	}
	longestValueLength := 0
	for _, in := range values {
		if len(in) > longestValueLength {
			longestValueLength = len(in)
		}
	}
	reps := []string{}
	for _, v := range values {
		reps = append(reps, old+v, new+v)
	}
	replacer := strings.NewReplacer(reps...)
	_, err = CopyWithFrames(source, output, nil, longestValueLength, func(b []byte) error {
		c := replacer.Replace(string(b))
		copy(b, c)
		return nil
	})
	return
}

// CopyWithFrames copies data between two sources. As data is copied it is
// loaded into a byte buffer that overlaps with the previous buffer. This
// ensures that bytes of a certain width will not be split over the boundary of
// a frame.
func CopyWithFrames(src io.Reader, dst io.Writer, buf []byte, overlapSize int, transform func(b []byte) error) (
	written int64, err error) {
	if buf == nil {
		size := 32 * 1024 // The default from io/io.go.
		buf = make([]byte, size)
	}
	firstPassOffset := overlapSize
	for {
		nr, er := src.Read(buf[overlapSize:])
		// TODO: if our first read is less than the length of the frame, what
		// do we do about that?
		// TODO: set up tests with many variable first read sizes
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
			if n := copy(buf[:overlapSize], buf[nr:nr+overlapSize]); n != overlapSize {
				panic(overlapSize)
			}
		} else if er != nil {
			if er != io.EOF {
				err = er
			}
			nw, ew := dst.Write(buf[:overlapSize])
			if ew != nil {
				err = ew
			}
			written += int64(nw)
			break
		} else {
			// if we have read 0 and there is no error just go back and read again
			continue
		}
		firstPassOffset = 0
	}
	return
}

func replaceBytesReplace(src io.Reader, dst io.Writer, old []byte, new []byte) (written int64, err error) {
	if len(old) != len(new) {
		return 0, ErrNotSameLength
	}
	return CopyWithFrames(src, dst, nil, len(old), func(b []byte) error {
		copy(b, bytes.ReplaceAll(b, old, new))
		return nil
	})
}

func ReplaceBytes(src io.Reader, dst io.Writer, old []byte, new []byte) (written int64, err error) {
	if len(old) != len(new) {
		return 0, ErrNotSameLength
	}
	return CopyWithFrames(src, dst, nil, len(old), func(b []byte) error {
		return InPlaceReplaceAll(b, old, new)
	})
}

func InPlaceReplaceAll(s, old, new []byte) (err error) {
	return InPlaceReplace(s, old, new, -1)
}

func InPlaceReplace(s, old, new []byte, n int) (err error) {
	if len(old) != len(new) {
		return ErrNotSameLength
	}

	m := 0
	if n != 0 {
		// Compute number of replacements.
		m = bytes.Count(s, old)
	}
	if m == 0 {
		// return unchanged
		return
	}
	if n < 0 || m < n {
		n = m
	}

	start := 0
	for i := 0; i < n; i++ {
		j := start
		j += bytes.Index(s[start:], old)
		copy(s[j:j+len(old)], new)
		start = j + len(old)
	}
	return nil
}
