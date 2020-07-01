package textreplace

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"

	"github.com/ThreeDotsLabs/watermill"
	radix "github.com/hashicorp/go-immutable-radix"
	"github.com/sirupsen/logrus"
)

var SomeRandomNixPaths = []string{
	"zzinxg9s1libj0k7gn7s4sl6zvvc2aja-libiec61883-1.2.0.tar.gz.drv",
	"zziylsdvcqgwwwhbspf1agbz0vldxjr3-perl5.30.2-JSON-4.02",
	"zzjsli12acp352n9i5db89hy5nnvfsdw-bcrypt-3.1.7.tar.gz.drv",
	"zzl9m6p4qzczcyf4s73n8aczrdw2ws5r-libsodium-1.0.18.tar.gz.drv",
	"zznqyjs1mz3i4ipg7cfjn0mki9ca9jvk-libxml2-2.9.10-bin",
	"zzp24m9jbrlqjp1zqf5n3s95jq6fhiqy-python3.7-python3.7-pytest-runner-4.4.drv",
	"zzsfwzjxvkvp3qmak8pwi05z99hihyng-curl-7.64.0.drv",
	"zzw8bb3ihq0jwhmy1mvcf48c3k627xbs-ghc-8.6.5-binary.drv",
	"zzxz9hl1zxana8f66jr7d7xkdhx066pm-xterm-353.drv",
	"zzy2an4hplsl06dfl6dgik4zmn7vycvd-hscolour-1.24.4.drv",
}

func prepNixPaths(prefix string, v []string) (out [][]byte) {
	for _, path := range v {
		out = append(out, []byte(prefix+path))
	}
	return
}

func GenerateUninterruptedCorpus(values [][]byte, count int) io.Reader {
	var buf bytes.Buffer
	for i := 0; i < count; i++ {
		_, _ = buf.Write(values[i%len(values)])
	}
	return bytes.NewReader(buf.Bytes())
}

func GenerateRandomCorpus(values [][]byte) io.Reader {
	count := 50
	chunks := count / (len(values))
	valueIndex := 0
	var buf bytes.Buffer
	b := make([]byte, 1024)
	for i := 0; i < count; i++ {
		rand.Read(b)
		if i%chunks == 0 {
			valueLen := len(values[valueIndex])
			// input can't be larger than the byte array
			// will panic if input size is larger than the byte array
			copy(b[0:valueLen], values[valueIndex])
			valueIndex++
		}
		buf.Write(b)
	}
	return bytes.NewReader(buf.Bytes())
}

type Replacer struct {
	currentPrefix   []byte
	prefixToReplace []byte
	values          [][]byte
	source          io.Reader
	matches         [][]byte
	tree            *radix.Tree

	longestValueLength int
	bufferOverflow     []byte

	replacements int
}

func NewReplacer(values [][]byte, prefixToReplace []byte, source io.Reader) *Replacer {
	r := Replacer{
		values:          values,
		prefixToReplace: prefixToReplace,
		source:          source,
		currentPrefix:   []byte("/nix/store/"),
	}
	tree := radix.New()
	for _, in := range r.values {
		if len(in) > r.longestValueLength {
			r.longestValueLength = len(in)
		}
		tree, _, _ = tree.Insert(in, struct{}{})
	}
	r.bufferOverflow = make([]byte, r.longestValueLength)
	r.tree = tree
	return &r
}

func (r *Replacer) NextValue(prefix []byte) (value []byte, found bool) {
	if len(prefix) == 0 {
		value, _, found = r.tree.Root().Minimum()
		return
	}
	r.tree.Root().WalkPrefix(prefix, func(k []byte, _ interface{}) bool {
		value = k
		found = true
		return true // should stop execution
	})
	return
}

func (r *Replacer) Read(b []byte) (n int, err error) {
	if len(r.bufferOverflow) == 0 {
		r.bufferOverflow = make([]byte, r.longestValueLength)
	} else {
		copy(b[:r.longestValueLength-1], r.bufferOverflow)
	}
	n, err = r.source.Read(b[r.longestValueLength:])
	j := 0
	for {
	BEGIN:
		i := bytes.Index(b[j:], r.currentPrefix)
		if i < 0 {
			break
		}
		j += i
		for _, input := range r.values {
			if len(input) > len(b[j:]) {
				continue
			}
			if bytes.Equal(b[j:j+len(input)], input) {
				r.replacements++
				copy(b[j:j+len(r.prefixToReplace)], r.prefixToReplace)
				goto BEGIN
			}
		}
		break
	}
	copy(r.bufferOverflow, b[len(b)-r.longestValueLength:])
	if n > 0 {
		n -= r.longestValueLength
	}
	return
}

func (r *Replacer) Read2(b []byte) (n int, err error) {
	n, err = r.source.Read(b)

	letterIndex := 0
	firstValue, found := r.NextValue([]byte{})
	value := firstValue
	if !found {
		return n, err
	}
	for i := 0; i < n; i++ {
		if b[i] == value[letterIndex] {
			if letterIndex == len(value)-1 {
				copy(b[i-letterIndex:i-letterIndex+len(r.prefixToReplace)], r.prefixToReplace)
				letterIndex = 0
				continue
			}
			letterIndex++
			continue
		} else if letterIndex > 0 {
			value, found = r.NextValue(b[i-letterIndex : i+1])
			if !found {
				value = firstValue
				letterIndex = 0
				continue
			}
			letterIndex++
		}
	}
	return n, err
}

func Run() {
	input := prepNixPaths("/nix/store/", SomeRandomNixPaths)

	prefixToReplace := "/tmp/wings/"

	corpus := GenerateRandomCorpus(input)
	r := NewReplacer(input, []byte(prefixToReplace), corpus)
	_, _ = ioutil.ReadAll(r)
}

// Framereader reads with an overlapping frame of bytes.
type FrameReader struct {
	FrameSize    int
	Reader       io.Reader
	buf          *bytes.Buffer
	ProcessFrame func(b []byte) error
	previousN    int
	err          error
}

func ReadyByFrame(frameSize int, reader io.Reader, p func(b []byte) error) io.Reader {
	return &FrameReader{
		FrameSize:    frameSize,
		Reader:       reader,
		ProcessFrame: p,
	}
}

func CopyWithFrames(dst io.Writer, src io.Reader, frameSize int, transform func(b []byte) error) (written int64, err error) {
	size := 15 // The default from io/io.go.
	buf := make([]byte, size)

	// frameIndex := 0
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			if err = transform(buf); err != nil {
				return
			}
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
		// frameIndex = frameSize
	}
	return
}

func (fr *FrameReader) Read(b []byte) (n int, err error) {
	if fr.err != nil {
		return 0, fr.err
	}
	// If it's the first read we fill up the whole buffer.
	if fr.buf == nil {
		fr.buf = &bytes.Buffer{}
		n, err = fr.Reader.Read(b)
		// We subtract the framesize from the output length so that
		// only a subset of the bytes is read.
		n -= fr.FrameSize
		fr.previousN = n
	} else {
		// Subsequent reads add overlapping bytes to the beginning.
		n, err = fr.Reader.Read(b[fr.FrameSize:])
		_, _ = fr.buf.Read(b[:fr.FrameSize])
	}

	// Do work on this frame. Fixed size replacements here will work
	// if the replacement size is < fr.FrameSize.
	if err := fr.ProcessFrame(b); err != nil {
		return 0, err
	}

	// Write the end bytes into our buffer.
	fr.buf.Write(b[len(b)-fr.FrameSize:])

	// If we're io.EOF we need to subtract the FrameSize from our output
	// to ensure the final bytes are read and then error on the next
	// Read call.
	if err != nil {
		fr.err = err
		return fr.previousN - fr.FrameSize, nil
	}
	// Use the previousN as our n might be from the final read before io.EOF
	// and would lead to a partial read on a full(er) buffer.
	n, fr.previousN = fr.previousN, n
	fmt.Println(n, len(b))
	return n, err
}

type Logger struct {
	log logrus.Logger
}

func (l Logger) Debug(msg string, fields watermill.LogFields) {
	l.log.WithFields(logrus.Fields(fields)).Debug(msg)

}
