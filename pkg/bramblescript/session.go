package bramblescript

import (
	"errors"
	"fmt"
	"strings"

	"go.starlark.net/starlark"
)

type Session struct {
	cmd *Cmd
	env map[string]string
}

var (
	_ starlark.Value    = new(Session)
	_ starlark.HasAttrs = new(Session)
)

func (session *Session) String() string {
	s := fmt.Sprintf
	var sb strings.Builder
	sb.WriteString("<session")
	sb.WriteString(s(" '%s'", session.cmd.name()))
	sb.WriteString(">")
	return sb.String()
}
func (session *Session) Type() string          { return "session" }
func (session *Session) Freeze()               {}
func (session *Session) Truth() starlark.Bool  { return session.cmd.Truth() }
func (session *Session) Hash() (uint32, error) { return 0, errors.New("session is unhashable") }

func (session *Session) Attr(name string) (val starlark.Value, err error) {
	switch name {
	case "cmd":
		return Callable{ThisName: "cmd", ParentName: "session", Callable: session.cmdMethod}, nil
	case "unset":
		return Callable{ThisName: "unset", ParentName: "session", Callable: session.unsetMethod}, nil
	case "env":
		return Callable{ThisName: "env", ParentName: "session", Callable: session.envMethod}, nil
	}
	return nil, nil
}
func (session *Session) AttrNames() []string {
	return []string{"env", "unset", "cmd"}
}

// StarlarkSession defines the session() starlark function.
func StarlarkSesssion(thread *starlark.Thread, fn *starlark.Builtin,
	args starlark.Tuple, kwargsList []starlark.Tuple) (v starlark.Value, err error) {
	return &Session{}, nil
}

func (session *Session) cmdMethod(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	if session.cmd != nil {
		if err = session.cmd.Wait(); err != nil {
			return
		}
	}
	session.cmd, err = NewCmd(args, kwargs, nil)
	return session.cmd, err
}

func (session *Session) unsetMethod(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var key starlark.Value
	if err = starlark.UnpackArgs("env", args, kwargs, "key", &key); err != nil {
		return
	}

	keyString, err := valueToString(key)
	if err != nil {
		return
	}
	delete(session.env, keyString)
	return starlark.None, nil
}

func (session *Session) envMethod(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var key starlark.Value
	var value starlark.Value
	if err = starlark.UnpackArgs("env", args, kwargs, "key", &key, "value", &value); err != nil {
		return
	}

	keyString, err := valueToString(key)
	if err != nil {
		return
	}
	valueString, err := valueToString(value)
	if err != nil {
		return
	}
	session.env[keyString] = valueString
	return starlark.None, nil
}
