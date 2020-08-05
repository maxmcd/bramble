package bramblescript

import (
	"io"
	"io/ioutil"
	"sync"

	"github.com/moby/moby/pkg/stdcopy"
)

type StandardStream struct {
	stdout bool
	stderr bool
	once   sync.Once

	buffPipe *bufferedPipe

	reader *io.PipeReader

	err  error
	lock sync.Mutex

	started bool
}

func NewStandardStream() (ss *StandardStream, stdoutWriter, stderrWriter io.Writer) {
	ss = &StandardStream{
		buffPipe: newBufferedPipe(4096 * 16),
	}

	stdoutWriter = stdcopy.NewStdWriter(ss.buffPipe, stdcopy.Stdout)
	stderrWriter = stdcopy.NewStdWriter(ss.buffPipe, stdcopy.Stderr)

	return
}

func (ss *StandardStream) Close() (err error) {
	_ = ss.buffPipe.Close()
	return nil
}

func (ss *StandardStream) Read(p []byte) (n int, err error) {
	ss.once.Do(func() {
		ss.started = true
		var writer *io.PipeWriter
		ss.lock.Lock()
		ss.reader, writer = io.Pipe()
		ss.lock.Unlock()
		var stdoutWriter io.Writer = ioutil.Discard
		var stderrWriter io.Writer = ioutil.Discard
		if ss.stdout {
			stdoutWriter = writer
		}
		if ss.stderr {
			stderrWriter = writer
		}
		go func() {
			_, err := stdcopy.StdCopy(stdoutWriter, stderrWriter, ss.buffPipe)
			ss.lock.Lock()
			ss.err = err
			ss.lock.Unlock()
			_ = writer.Close()
			_ = ss.reader.Close()
		}()
	})
	n, err = ss.reader.Read(p)
	ss.lock.Lock()
	if err == nil && ss.err != nil {
		err = ss.err
	}
	ss.lock.Unlock()
	return
}
