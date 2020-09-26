package bramble

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/kballard/go-shellquote"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/moby/moby/pkg/stdcopy"
	"github.com/pkg/errors"

	"go.starlark.net/starlark"
)

var ErrInvalidRead = errors.New("can't read from command output more than once")

// CmdFunction is the value for the builtin "cmd", calling it as a function
// creates a new cmd instance, it also has various other attributes and methods
type CmdFunction struct {
	session *session

	bramble *Bramble

	// derivations that are touched by running commands
	inputDerivations DerivationOutputs
}

var (
	_ starlark.Value    = new(CmdFunction)
	_ starlark.Callable = new(CmdFunction)
)

func NewCmdFunction(session *session, bramble *Bramble) *CmdFunction {
	return &CmdFunction{session: session, bramble: bramble}
}

func (fn *CmdFunction) Freeze()               {}
func (fn *CmdFunction) Hash() (uint32, error) { return 0, starutil.ErrUnhashable(fn.Type()) }
func (fn *CmdFunction) Name() string          { return fn.Type() }
func (fn *CmdFunction) String() string        { return "<built-in function cmd>" }
func (fn *CmdFunction) Type() string          { return "builtin_function_cmd" }
func (fn *CmdFunction) Truth() starlark.Bool  { return true }

// CallInternal defines the cmd() starlark function.
func (fn *CmdFunction) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	if isTopLevel(thread) {
		return nil, errors.New("cmd call not within a function")
	}

	return fn.newCmd(thread, args, kwargs, nil)
}

func (cmd *Cmd) searchForDerivationOutputs() DerivationOutputs {
	var buf bytes.Buffer
	fmt.Fprint(&buf, cmd.Path)
	fmt.Fprint(&buf, strings.Join(cmd.Args, ""))
	fmt.Fprint(&buf, strings.Join(cmd.Env, ""))

	return searchForDerivationOutputs(buf.String())
}

func cmdArgumentsFromArgs(args starlark.Tuple) (out []string, err error) {
	// it's cmd(["grep", "-v"])
	if args.Len() == 1 {
		if iterable, ok := args.Index(0).(*starlark.List); ok {
			if out, err = starutil.IterableToGoList(iterable); err != nil {
				return
			} else if len(out) == 0 {
				return nil, errors.New("if the first argument is a list it can't be empty")
			}
			return
		}
		var str string
		if str, err = starutil.ValueToString(args.Index(0)); err != nil {
			return
		}
		if str == "" {
			return nil, errors.New("if the first argument is a string it can't be empty")
		}

		out, err = shellquote.Split(str)
		if len(out) == 0 {
			// whitespace bash characters will be removed by shellquote,
			// add them back for correct error message
			return []string{str}, nil
		}
		return
	}
	return starutil.IterableToGoList(args)
}

// NewCmd creates a new cmd instance given args and kwargs. NewCmd will error
// immediately if it can't find the cmd
func (fn *CmdFunction) newCmd(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple, stdin *Cmd) (val starlark.Value, err error) {
	cmd := Cmd{fn: fn}
	cmd.Dir = fn.session.currentDirectory

	var stdinKwarg starlark.Value
	var dirKwarg starlark.String
	var envKwarg *starlark.Dict
	var clearEnvKwarg starlark.Bool
	var ignoreFailureKwarg starlark.Bool
	var printOutputKwarg starlark.Bool
	if err = starlark.UnpackArgs("f", nil, kwargs,
		"stdin?", &stdinKwarg,
		"dir?", &dirKwarg,
		"env?", &envKwarg,
		"clear_env?", &clearEnvKwarg,
		"ignore_failure?", &ignoreFailureKwarg,
		"print_output?", &printOutputKwarg,
	); err != nil {
		return
	}
	if clearEnvKwarg == starlark.True {
		cmd.Env = []string{}
	} else {
		cmd.Env = fn.session.envArray()
	}
	if envKwarg != nil {
		kvs, err := starutil.DictToGoStringMap(envKwarg)
		if err != nil {
			return nil, err
		}
		for k, v := range kvs {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	// this feels like it's helping determinism, is it?
	sort.Strings(cmd.Env)

	// and empty cmd() call isn't allowed
	if args.Len() == 0 {
		return nil, errors.New("cmd() missing 1 required positional argument")
	}

	if args.Index(0).Type() == "function" {
		fn := args.Index(0).(callInternalable)
		kwargs = append(kwargs, starlark.Tuple{starlark.String("stdin"), stdin})
		return fn.CallInternal(thread, args[1:], kwargs)
	}
	cmd.Args, err = cmdArgumentsFromArgs(args)
	if err != nil {
		return
	}

	// +------------------------------------+
	// | Basic input validation is complete |
	// +------------------------------------+

	// Search for input derivations

	dos := cmd.searchForDerivationOutputs()
	fn.inputDerivations = append(fn.inputDerivations, dos...)
	if err = fn.bramble.buildDerivationOutputs(dos); err != nil {
		return
	}

	if err = fn.bramble.replaceOutputValuesInCmd(&cmd); err != nil {
		return
	}

	if stdinKwarg != nil {
		if err = cmdAttachStdin(&cmd, stdinKwarg); err != nil {
			return
		}
	}
	name := cmd.name()
	if filepath.Base(name) == name {
		var lp string
		if lp, err = lookPath(name, cmd.path()); err != nil {
			return nil, err
		}
		cmd.Path = lp
	} else {
		cmd.Path = name
	}
	fmt.Println(cmd.PythonFunctionString())
	cmd.wg = &sync.WaitGroup{}
	cmd.wg.Add(1)

	cmd.ss, cmd.Stdout, cmd.Stderr = NewStandardStream()
	if printOutputKwarg {
		cmd.Stdout = io.MultiWriter(cmd.Stdout, os.Stdout)
		cmd.Stderr = io.MultiWriter(cmd.Stderr, os.Stderr)
	}

	if stdin != nil {
		cmd.Stdin = stdin
	}

	err = cmd.Start()
	go func() {
		err := cmd.Cmd.Wait()
		{
			cmd.lock.Lock()
			cmd.processState = cmd.ProcessState
			if err != nil {
				cmd.err = err
			}
			cmd.finished = true
			cmd.lock.Unlock()
		}
		cmd.ss.Close()
		cmd.wg.Done()
	}()
	return &cmd, err
}

type Cmd struct {
	exec.Cmd

	fn       *CmdFunction
	frozen   bool
	finished bool
	err      error

	lock sync.Mutex

	processState *os.ProcessState

	wg *sync.WaitGroup

	ss *StandardStream
}

var (
	_ starlark.Value    = new(Cmd)
	_ starlark.HasAttrs = new(Cmd)
)

func (cmd *Cmd) name() string {
	if cmd == nil || len(cmd.Args) == 0 {
		return ""
	}
	return cmd.Args[0]
}

func (cmd *Cmd) String() string {
	s := fmt.Sprintf
	var sb strings.Builder
	sb.WriteString("<cmd")
	sb.WriteString(s(" '%s'", cmd.name()))
	if len(cmd.Args) > 1 {
		sb.WriteString(" ['")
		sb.WriteString(strings.Join(cmd.Args[1:], `', '`))
		sb.WriteString("']")
	}
	sb.WriteString(">")
	return sb.String()
}

func (cmd *Cmd) PythonFunctionString() string {
	s := fmt.Sprintf
	var sb strings.Builder
	sb.WriteString("cmd(")
	sb.WriteString(s("%q", cmd.name()))
	if len(cmd.Args) > 1 {
		sb.WriteString(` "`)
		sb.WriteString(strings.Join(cmd.Args[1:], `", "`))
		sb.WriteString(`"`)
	}
	sb.WriteString(")")
	return sb.String()
}
func (cmd *Cmd) Freeze() {
	// TODO: don't implement functionality that does nothing
	if cmd != nil {
		cmd.frozen = true
	}
}
func (cmd *Cmd) Type() string          { return "cmd" }
func (cmd *Cmd) Truth() starlark.Bool  { return cmd != nil }
func (cmd *Cmd) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("cmd") }

func (cmd *Cmd) path() string {
	for _, ev := range cmd.Env {
		p := strings.SplitN(ev, "=", 2)
		if p[0] == "PATH" && len(p) == 2 {
			return p[1]
		}
	}
	return ""
}

func (cmd *Cmd) Attr(name string) (val starlark.Value, err error) {
	switch name {
	case "output":
		return ByteStream{stdout: true, stderr: true, cmd: cmd}, nil
	case "exit_code":
		return cmd.ExitCode(), nil
	case "if_err":
		return starutil.Callable{ThisName: "if_err", ParentName: "cmd", Callable: cmd.IfErr}, nil
	case "kill":
		return starutil.Callable{ThisName: "kill", ParentName: "cmd", Callable: cmd.Kill}, nil
	case "pipe":
		return starutil.Callable{ThisName: "pipe", ParentName: "cmd", Callable: cmd.Pipe}, nil
	case "stderr":
		return ByteStream{stderr: true, cmd: cmd}, nil
	case "stdout":
		return ByteStream{stdout: true, cmd: cmd}, nil
	case "wait":
		return starutil.Callable{ThisName: "wait", ParentName: "cmd", Callable: cmd.starlarkWait}, nil
	}
	return nil, nil
}

func (cmd *Cmd) AttrNames() []string {
	return []string{
		"output",
		"exit_code",
		"if_err",
		"kill",
		"pipe",
		"stderr",
		"stdout",
		"wait",
	}
}

func (cmd *Cmd) ExitCode() starlark.Value {
	cmd.lock.Lock()
	defer cmd.lock.Unlock()
	if cmd.processState != nil {
		code := cmd.processState.ExitCode()
		if code > -1 {
			return starlark.MakeInt(code)
		}
	}
	return starlark.None
}

func (cmd *Cmd) Wait() error {
	cmd.wg.Wait()
	return cmd.err
}

func (cmd *Cmd) IfErr(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	// If there's no error we ignore the 'or' call
	if err := cmd.Wait(); err == nil {
		return cmd, nil
	}

	// if there is an error we run the command in or instead
	return cmd.fn.newCmd(thread, args, kwargs, nil)
}

func (cmd *Cmd) Pipe(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	_ = cmd.setOutput(true, false)
	return cmd.fn.newCmd(thread, args, kwargs, cmd)
}

func (cmd *Cmd) starlarkWait(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	return cmd, cmd.Wait()
}

func (cmd *Cmd) Kill(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	if err = starlark.UnpackArgs("kill", args, kwargs); err != nil {
		return
	}
	val = starlark.None
	cmd.lock.Lock()
	if cmd.finished {
		return
	}
	if cmd.Process != nil {
		if err = cmd.Process.Kill(); err != nil {
			return
		}
	}
	cmd.lock.Unlock()
	return
}

type callInternalable interface {
	CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error)
}

func cmdAttachStdin(cmd *Cmd, val starlark.Value) (err error) {
	switch v := val.(type) {
	case *Cmd:
		if err = v.setOutput(true, false); err != nil {
			return
		}
		cmd.Stdin = v
	case ByteStream:
		if err = v.cmd.setOutput(v.stdout, v.stderr); err != nil {
			return
		}
		cmd.Stdin = v.cmd
	case starlark.String:
		cmd.Stdin = bytes.NewBufferString(v.GoString())
	default:
		return errors.Errorf("can't take type %t for stdin", v)
	}
	return nil
}

func (cmd *Cmd) setOutput(stdout, stderr bool) (err error) {
	if cmd.ss.started {
		return ErrInvalidRead
	}
	cmd.ss.stdout = stdout
	cmd.ss.stderr = stderr
	return
}

func (cmd *Cmd) Read(p []byte) (n int, err error) {
	n, err = cmd.ss.Read(p)
	cmd.lock.Lock()
	defer cmd.lock.Unlock()
	if cmd.err != nil {
		return n, cmd.err
	}

	return
}

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
	if err != nil {
		// TODO: need a better solution for this
		fmt.Println(string(b))
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
