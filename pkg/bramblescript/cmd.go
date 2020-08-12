package bramblescript

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kballard/go-shellquote"
	"github.com/pkg/errors"

	"go.starlark.net/starlark"
)

type Cmd struct {
	exec.Cmd
	frozen   bool
	finished bool
	err      error

	lock sync.Mutex

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
func (cmd *Cmd) Freeze() {
	// TODO: don't implement functionality that does nothing
	if cmd != nil {
		cmd.frozen = true
	}
}
func (cmd *Cmd) Type() string          { return "cmd" }
func (cmd *Cmd) Truth() starlark.Bool  { return cmd != nil }
func (cmd *Cmd) Hash() (uint32, error) { return 0, errors.New("cmd is unhashable") }

func (cmd *Cmd) Attr(name string) (val starlark.Value, err error) {
	switch name {
	case "combined_output":
		return ByteStream{stdout: true, stderr: true, cmd: cmd}, nil
	case "exit_code":
		return cmd.ExitCode(), nil
	case "if_err":
		return IfErr{cmd: cmd}, nil
	case "kill":
		return Callable{ThisName: "kill", ParentName: "cmd", Callable: cmd.Kill}, nil
	case "pipe":
		return Pipe{cmd: cmd}, nil
	case "stderr":
		return ByteStream{stderr: true, cmd: cmd}, nil
	case "stdout":
		return ByteStream{stdout: true, cmd: cmd}, nil
	case "wait":
		return Callable{ThisName: "wait", ParentName: "cmd", Callable: cmd.starlarkWait}, nil
	}
	return nil, nil
}
func (cmd *Cmd) AttrNames() []string {
	return []string{
		"combined_output",
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
	if cmd.ProcessState != nil {
		code := cmd.ProcessState.ExitCode()
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

func (cmd *Cmd) addArgumentToCmd(value starlark.Value) (err error) {
	val, err := valueToString(value)
	if err != nil {
		return
	}
	cmd.Args = append(cmd.Args, val)
	return
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

func attachStdin(cmd *Cmd, val starlark.Value) (err error) {
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

// NewCmd creates a new cmd instance given args and kwargs. NewCmd will error
// immediately if it can't find the cmd
func newCmd(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple, stdin *Cmd, dir string) (val starlark.Value, err error) {
	// if input is an array we use the first item as the cmd
	// if input is just args we use them as cmd+args
	// if input is just a string we parse it as a shell command

	cmd := Cmd{}
	cmd.Dir = dir

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
	}
	if envKwarg != nil {
		for _, key := range envKwarg.Keys() {
			envVal, _, _ := envKwarg.Get(key)
			keyString, err := valueToString(key)
			if err != nil {
				return nil, err
			}
			valString, err := valueToString(envVal)
			if err != nil {
				return nil, err
			}
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", keyString, valString))
		}
	}

	// cmd() isn't allowed
	if args.Len() == 0 {
		return nil, errors.New("cmd() missing 1 required positional argument")
	}

	if args.Index(0).Type() == "function" {
		fn := args.Index(0).(callInternalable)
		kwargs = append(kwargs, starlark.Tuple{starlark.String("stdin"), stdin})
		return fn.CallInternal(thread, args[1:], kwargs)
	}

	// it's cmd(["grep", "-v"])
	if args.Len() == 1 {
		if args.Index(0).Type() == "list" {
			cmd.Args, err = starlarkListToListOfStrings(args.Index(0))
			if err != nil {
				return nil, err
			}
			if len(cmd.Args) == 0 {
				return nil, errors.New("if the first argument is a list it can't be empty")
			}
		} else if args.Index(0).Type() == "string" {
			starlarkCmd := args.Index(0).(starlark.String).GoString()
			if starlarkCmd == "" {
				return nil, errors.New("if the first argument is a string it can't be empty")
			}
			cmd.Args, err = shellquote.Split(starlarkCmd)
			if err != nil {
				return
			}
			if len(cmd.Args) == 0 {
				// whitespace bash characters will be removed by shellquote,
				// add them back for correct error message
				cmd.Args = []string{starlarkCmd}
			}
		}
	} else {
		iterator := args.Iterate()
		defer iterator.Done()
		var val starlark.Value
		for iterator.Next(&val) {
			if err := cmd.addArgumentToCmd(val); err != nil {
				return nil, err
			}
		}
	}

	if stdinKwarg != nil {
		if err = attachStdin(&cmd, stdinKwarg); err != nil {
			return
		}
	}
	// kwargs:
	// stdin
	// dir
	// env
	name := cmd.name()
	if filepath.Base(name) == name {
		var lp string
		if lp, err = exec.LookPath(name); err != nil {
			return nil, err
		}
		cmd.Path = lp
	}

	cmd.wg = &sync.WaitGroup{}
	cmd.wg.Add(1)

	cmd.ss, cmd.Stdout, cmd.Stderr = NewStandardStream()
	if stdin != nil {
		cmd.Stdin = stdin
	}
	err = cmd.Start()
	logger.Println(cmd.String(), "started")
	go func() {
		err := cmd.Cmd.Wait()
		{
			cmd.lock.Lock()
			if err != nil {
				logger.Println(cmd.String(), "error", err)
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
