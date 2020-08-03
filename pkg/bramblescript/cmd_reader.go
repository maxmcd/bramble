package bramblescript

import (
	"errors"
	"io"
	"io/ioutil"

	"github.com/moby/moby/pkg/stdcopy"
)

type cmdReader struct {
	cmd *Cmd
	rc  io.ReadCloser
}

func (cr *cmdReader) Read(b []byte) (n int, err error) {
	n, err = cr.rc.Read(b)
	if cr.cmd.err != nil {
		err = cr.cmd.err
	}
	return
}
func (cr *cmdReader) Close() (err error) {
	return cr.rc.Close()
}

type errReader struct {
	err error
	r   io.Reader
}

func (er *errReader) Read(p []byte) (n int, err error) {
	n, err = er.r.Read(p)
	if er.err != nil {
		err = er.err
	}
	return
}

func (cmd *Cmd) Reader(stdout, stderr bool) (io.Reader, error) {
	if cmd.reading {
		return nil, errors.New("can't read from command output twice")
	}
	cmd.reading = true
	reader, writer := io.Pipe()
	var stdoutWriter io.Writer = ioutil.Discard
	var stderrWriter io.Writer = ioutil.Discard
	if stdout {
		stdoutWriter = writer
	}
	if stderr {
		stderrWriter = writer
	}
	er := &errReader{r: reader}
	go func() {
		_, err := stdcopy.StdCopy(stdoutWriter, stderrWriter, &cmdReader{rc: cmd.out, cmd: cmd})
		er.err = err
		_ = writer.Close()
		_ = reader.Close()
	}()
	return er, nil
}
