package bramblecmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"

	"go.starlark.net/starlark"
)

type ByteStream struct {
	cmd    *Cmd
	stdout bool
	stderr bool
}

var (
	_ starlark.Value    = ByteStream{}
	_ starlark.Callable = ByteStream{}
	_ starlark.Iterable = ByteStream{}
)

func (bs ByteStream) Name() string {
	if bs.stdout && bs.stderr {
		return "output"
	}
	if bs.stderr {
		return "stderr"
	}
	return "stdout"
}
func (bs ByteStream) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	if err = bs.cmd.setOutput(bs.stdout, bs.stderr); err != nil {
		return
	}
	b, err := ioutil.ReadAll(bs.cmd)
	if err == io.ErrClosedPipe {
		err = nil
	}
	return starlark.String(string(b)), err
}

func (bs ByteStream) String() string {
	return fmt.Sprintf("<attribute '%s' of 'cmd'>", bs.Name())
}

func (bs ByteStream) Type() string          { return "bytestream" }
func (bs ByteStream) Freeze()               {}
func (bs ByteStream) Truth() starlark.Bool  { return bs.cmd.Truth() }
func (bs ByteStream) Hash() (uint32, error) { return 0, errors.New("bytestream is unhashable") }

func (bs ByteStream) Iterate() starlark.Iterator {
	err := bs.cmd.setOutput(bs.stdout, bs.stderr)
	bsi := byteStreamIterator{
		bs: bs,
	}
	if err == nil {
		bsi.buf = bufio.NewReader(bs.cmd)
	}
	return bsi
}

type byteStreamIterator struct {
	bs  ByteStream
	buf *bufio.Reader
}

func (bsi byteStreamIterator) Next(p *starlark.Value) bool {
	if bsi.buf == nil {
		return false
	}
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
