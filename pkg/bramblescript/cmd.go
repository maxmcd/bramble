package bramblescript

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kballard/go-shellquote"
	"github.com/moby/moby/pkg/stdcopy"
	"go.starlark.net/starlark"
)

type Cmd struct {
	exec.Cmd
	name   string
	frozen bool

	out *bytes.Buffer
}

func (c *Cmd) String() string {
	s := fmt.Sprintf
	var sb strings.Builder
	sb.WriteString("<cmd")
	sb.WriteString(s(" '%s'", c.name))
	if len(c.Args) > 0 {
		sb.WriteString(" ['")
		sb.WriteString(strings.Join(c.Args, `', '`))
		sb.WriteString("']")
	}
	sb.WriteString(">")
	return sb.String()
}
func (c *Cmd) Type() string          { return "cmd" }
func (c *Cmd) Freeze()               { c.frozen = false }
func (c *Cmd) Truth() starlark.Bool  { return c != nil }
func (c *Cmd) Hash() (uint32, error) { return 0, errors.New("cmd is unhashable") }

func (c *Cmd) stdout() (stdout starlark.String, err error) {
	var buf bytes.Buffer
	_, err = stdcopy.StdCopy(&buf, ioutil.Discard, c.out)
	return starlark.String(buf.String()), err
}

func (c *Cmd) stderr() (stderr starlark.String, err error) {
	var buf bytes.Buffer
	_, err = stdcopy.StdCopy(ioutil.Discard, &buf, c.out)
	return starlark.String(buf.String()), err
}

func (c *Cmd) combinedOutput() (combined starlark.String, err error) {
	var buf bytes.Buffer
	_, err = stdcopy.StdCopy(&buf, &buf, c.out)
	return starlark.String(buf.String()), err
}

func (c *Cmd) addArgumentToCmd(count int, value starlark.Value) (err error) {
	var stringValue string
	switch v := value.(type) {
	case starlark.String:
		stringValue = v.GoString()
	}

	if count == 0 {
		c.name = stringValue
	} else {
		c.Args = append(c.Args, stringValue)
	}
	return nil
}

// StarlarkCmd defines the cmd() starlark function.
func StarlarkCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargsList []starlark.Tuple) (v starlark.Value, err error) {
	// if input is an array we use the first item as the cmd
	// if input is just args we use them as cmd+args
	// if input is just a string we parse it as a shell command

	cmd := Cmd{}
	// TODO: might want CommandContext

	kwargs := map[string]starlark.Value{}
	for _, kwarg := range kwargsList {
		kwargs[kwarg.Index(0).(*starlark.String).GoString()] = kwarg.Index(1)
	}

	// cmd() isn't allowed
	if args.Len() == 0 {
		return nil, errors.New("cmd() missing 1 required positional argument")
	}

	// it's cmd(["grep", "-v"])
	if args.Len() == 1 {
		var words []string
		if args.Index(0).Type() == "list" {
			words, err = starlarkListToListOfStrings(args.Index(0))
			if err != nil {
				return nil, err
			}
			if len(words) == 0 {
				return nil, errors.New("if the first argument is a list it can't be empty")
			}
		} else if args.Index(0).Type() == "string" {
			starlarkCmd := args.Index(0).(starlark.String).GoString()
			if starlarkCmd == "" {
				return nil, errors.New("if the first argument is a string it can't be empty")
			}
			words, err = shellquote.Split(starlarkCmd)
			if err != nil {
				return
			}
			if len(words) == 0 {
				// whitespace bash characters will be removed by shellquote,
				// add them back for correct error message
				words = append(words, starlarkCmd)
			}
		}
		cmd.name = words[0]
		if len(words) > 1 {
			cmd.Args = words[1:]
		}
	} else {
		iterator := args.Iterate()
		defer iterator.Done()
		var count int
		var val starlark.Value
		for iterator.Next(&val) {
			if err := cmd.addArgumentToCmd(count, val); err != nil {
				return nil, err
			}
			count++
		}
	}

	// kwargs:
	// stdin
	// dir
	// env
	if filepath.Base(cmd.name) == cmd.name {
		var lp string
		if lp, err = exec.LookPath(cmd.name); err != nil {
			return nil, err
		}
		cmd.Path = lp
	}
	cmd.out = bytes.NewBuffer(nil)

	cmd.Stdout = stdcopy.NewStdWriter(cmd.out, stdcopy.Stdout)
	cmd.Stderr = stdcopy.NewStdWriter(cmd.out, stdcopy.Stderr)
	return &cmd, cmd.Start()
}

// func starlarkCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargsList []starlark.Tuple) (cmd Cmd, v starlark.Value, err error) {
// 	cmd, v, err = starlarkCmd(thread, fn, args, kwargsList)
// 	cmd.Stdout
// 	cmd.Run()
// 	return
// }
