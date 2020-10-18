package bramble

import (
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"sync"
	"syscall"

	"github.com/hashicorp/terraform/dag"
	"github.com/moby/moby/pkg/stdcopy"
)

type DerivationsMap struct {
	sync.Map
}

func (dm *DerivationsMap) Get(id string) *Derivation {
	d, ok := dm.Load(id)
	if !ok {
		return nil
	}
	return d.(*Derivation)
}

func (dm *DerivationsMap) Has(id string) bool {
	return dm.Get(id) != nil
}
func (dm *DerivationsMap) Set(id string, drv *Derivation) {
	dm.Store(id, drv)
}

// Range calls f sequentially for each key and value present in the map. If f
// returns false, range stops the iteration.
func (dm *DerivationsMap) Range(f func(filename string, drv *Derivation) bool) {
	dm.Map.Range(func(key, value interface{}) bool {
		return f(key.(string), value.(*Derivation))
	})
}

type AcyclicGraph struct {
	dag.AcyclicGraph
}

func NewAcyclicGraph() *AcyclicGraph {
	return &AcyclicGraph{}
}

func (ag AcyclicGraph) PrintDot() {
	graphString := string(ag.Dot(&dag.DotOpts{DrawCycles: true, Verbose: true}))
	fmt.Println(strings.ReplaceAll(graphString, "\"[root] ", "\""))
}

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

// bufferedPipe is a buffered pipe
// parts taken from https://github.com/golang/go/blob/0436b162397018c45068b47ca1b5924a3eafdee0/src/net/net_fake.go#L173
type bufferedPipe struct {
	softLimit int
	mu        sync.Mutex
	buf       []byte
	closed    bool
	rCond     sync.Cond
	wCond     sync.Cond
}

func newBufferedPipe(softLimit int) *bufferedPipe {
	p := &bufferedPipe{softLimit: softLimit}
	p.rCond.L = &p.mu
	p.wCond.L = &p.mu
	return p
}

func (p *bufferedPipe) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		if p.closed && len(p.buf) == 0 {
			return 0, io.EOF
		}
		if len(p.buf) > 0 {
			break
		}
		p.rCond.Wait()
	}

	n := copy(b, p.buf)
	p.buf = p.buf[n:]
	p.wCond.Broadcast()
	return n, nil
}

func (p *bufferedPipe) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for {
		if p.closed {
			return 0, syscall.ENOTCONN
		}
		if len(p.buf) <= p.softLimit {
			break
		}
		p.wCond.Wait()
	}

	p.buf = append(p.buf, b...)
	p.rCond.Broadcast()
	return len(b), nil
}

func (p *bufferedPipe) Close() (err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
	p.rCond.Broadcast()
	p.wCond.Broadcast()
	return nil
}

type BiStringMap struct {
	s       sync.RWMutex
	forward map[string]string
	inverse map[string]string
}

// NewBiStringMap returns a an empty, mutable, BiStringMap
func NewBiStringMap() *BiStringMap {
	return &BiStringMap{
		forward: make(map[string]string),
		inverse: make(map[string]string),
	}
}

func (b *BiStringMap) Store(k, v string) {
	b.s.Lock()
	b.forward[k] = v
	b.inverse[v] = k
	b.s.Unlock()
}

func (b *BiStringMap) Load(k string) (v string, exists bool) {
	b.s.RLock()
	v, exists = b.forward[k]
	b.s.RUnlock()
	return
}

func (b *BiStringMap) StoreInverse(k, v string) {
	b.s.Lock()
	b.forward[v] = k
	b.inverse[k] = v
	b.s.Unlock()
}

func (b *BiStringMap) LoadInverse(k string) (v string, exists bool) {
	b.s.RLock()
	v, exists = b.inverse[k]
	b.s.RUnlock()
	return
}
