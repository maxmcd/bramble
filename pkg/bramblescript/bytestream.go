package bramblescript

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/moby/moby/pkg/stdcopy"
	"go.starlark.net/starlark"
)

const (
	Stdout uint8 = 0x01
	Stderr uint8 = 0x02
)

type ByteStream struct {
	cmd  *Cmd
	kind uint8
}

var (
	_ starlark.Value    = ByteStream{}
	_ starlark.Callable = ByteStream{}
	_ starlark.Iterable = ByteStream{}
)

func (bs ByteStream) Name() string {
	if bs.kind&Stdout != 0 && bs.kind&Stderr != 0 {
		return "combined_output"
	}
	if bs.kind&Stderr != 0 {
		return "stderr"
	}
	return "stdout"
}
func (bs ByteStream) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	// TODO: ensure this is safe to call multiple times

	var buf bytes.Buffer
	var stdout io.Writer = ioutil.Discard
	var stderr io.Writer = ioutil.Discard
	if bs.kind&Stdout != 0 {
		stdout = &buf
	}
	if bs.kind&Stderr != 0 {
		stderr = &buf
	}
	_, err = stdcopy.StdCopy(stdout, stderr, bs.cmd.out)
	return starlark.String(buf.String()), err
}

func (bs ByteStream) String() string {
	return fmt.Sprintf("<attribute '%s' of 'cmd'>", bs.Name())
}

func (bs ByteStream) Type() string          { return "bytestream" }
func (bs ByteStream) Freeze()               {}
func (bs ByteStream) Truth() starlark.Bool  { return bs.cmd.Truth() }
func (bs ByteStream) Hash() (uint32, error) { return 0, errors.New("bytestream is unhashable") }

func (bs ByteStream) Iterate() starlark.Iterator {
	reader, writer := io.Pipe()
	var stdout io.Writer = ioutil.Discard
	var stderr io.Writer = ioutil.Discard
	if bs.kind&Stdout != 0 {
		stdout = writer
	}
	if bs.kind&Stderr != 0 {
		stderr = writer
	}
	go func() {
		_, _ = stdcopy.StdCopy(stdout, stderr, bs.cmd.out)
		_ = reader.Close()
		_ = writer.Close()
	}()
	return byteStreamIterator{
		bs:  bs,
		buf: bufio.NewReader(reader),
	}
}

type byteStreamIterator struct {
	bs  ByteStream
	buf *bufio.Reader
}

func (bsi byteStreamIterator) Next(p *starlark.Value) bool {
	str, err := bsi.buf.ReadString('\n')
	if err == io.EOF || err == io.ErrClosedPipe {
		return false
	}
	if err != nil {
		// TODO: something better here? certain errors we care about?
		panic(err)
	}

	*p = starlark.String(str[:len(str)-1])
	return true
}

func (bsi byteStreamIterator) Done() {}
