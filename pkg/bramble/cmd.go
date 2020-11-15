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
	"github.com/pkg/errors"

	"go.starlark.net/starlark"
)

var ErrInvalidRead = errors.New("can't read from command output more than once")

// CmdFunction is the value for the builtin "cmd", calling it as a function
// creates a new cmd instance, it also has various other attributes and methods
type CmdFunction struct {
	session *Session

	bramble *Bramble

	// derivations that are touched by running commands
	inputDerivations DerivationOutputs
}

var (
	_ starlark.Value    = new(CmdFunction)
	_ starlark.Callable = new(CmdFunction)
)

func NewCmdFunction(session *Session, bramble *Bramble) *CmdFunction {
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
	cmd := Cmd{fn: fn,
		Cmd: exec.Cmd{
			SysProcAttr: &syscall.SysProcAttr{
				Credential: fn.bramble.credentials,
			},
		},
	}
	cmd.Dir = fn.session.currentDirectory

	var stdinKwarg starlark.Value
	var dirKwarg starlark.String
	var envKwarg *starlark.Dict
	var ignoreFailureKwarg starlark.Bool
	var printOutputKwarg starlark.Bool
	if err = starlark.UnpackArgs("f", nil, kwargs,
		"stdin?", &stdinKwarg,
		"dir?", &dirKwarg,
		"env?", &envKwarg,
		"ignore_failure?", &ignoreFailureKwarg,
		"print_output?", &printOutputKwarg,
	); err != nil {
		return
	}
	cmd.ignoreErr = bool(ignoreFailureKwarg)
	cmd.Env = []string{}
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
				if !cmd.ignoreErr {
					cmd.err = err
				}
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

	fn        *CmdFunction
	frozen    bool
	finished  bool
	err       error
	ignoreErr bool

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
	callables := map[string]starutil.CallableFunc{
		"if_err": cmd.ifErr,
		"kill":   cmd.kill,
		"pipe":   cmd.pipe,
		"wait":   cmd.starlarkWait,
	}
	if fn, ok := callables[name]; ok {
		return starutil.Callable{ThisName: name, ParentName: "cmd", Callable: fn}, nil
	}
	switch name {
	case "exit_code":
		return cmd.ExitCode(), nil
	case "output":
		return ByteStream{stdout: true, stderr: true, cmd: cmd}, nil
	case "stderr":
		return ByteStream{stderr: true, cmd: cmd}, nil
	case "stdout":
		return ByteStream{stdout: true, cmd: cmd}, nil
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

func (cmd *Cmd) ifErr(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	// If there's no error we ignore the 'or' call
	if err := cmd.Wait(); err == nil {
		return cmd, nil
	}

	// if there is an error we run the command in or instead
	return cmd.fn.newCmd(thread, args, kwargs, nil)
}

func (cmd *Cmd) pipe(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	_ = cmd.setOutput(true, false)
	return cmd.fn.newCmd(thread, args, kwargs, cmd)
}

func (cmd *Cmd) starlarkWait(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	return cmd, cmd.Wait()
}

func (cmd *Cmd) kill(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
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

func (bs ByteStream) readAllBytes() (b []byte, err error) {
	if err = bs.cmd.setOutput(bs.stdout, bs.stderr); err != nil {
		return
	}
	b, err = ioutil.ReadAll(bs.cmd)
	if err == io.ErrClosedPipe {
		err = nil
	}
	if err != nil {
		// TODO: need a better solution for this
		fmt.Print(string(b))
	}
	return b, err
}

func (bs ByteStream) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	b, err := bs.readAllBytes()
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

func (bs ByteStream) Attr(name string) (val starlark.Value, err error) {
	if name == "to_file" {
		return starutil.Callable{
			ParentName: bs.Name(),
			ThisName:   name,
			Callable: func(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				b, err := bs.readAllBytes()
				if err != nil {
					return nil, err
				}

				var path starlark.String
				if err = starlark.UnpackArgs(name, args, kwargs, "path", &path); err != nil {
					return nil, err
				}
				pathString := path.GoString()
				if !filepath.IsAbs(pathString) {
					pathString = filepath.Join(bs.cmd.fn.session.currentDirectory, pathString)
				}
				f, err := os.Create(pathString)
				if err != nil {
					return nil, err
				}
				if _, err = f.Write(b); err != nil {
					return nil, err
				}
				return starlark.None, f.Close()
			},
		}, nil
	}
	return nil, nil
}

func (bs ByteStream) AttrNames() (out []string) {
	return []string{"to_file"}
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
