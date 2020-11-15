package bramble

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

type Session struct {
	env              map[string]string
	currentDirectory string
	frozen           bool
	bramble          *Bramble
}

func (b *Bramble) newSession(wd string, env map[string]string) (Session, error) {
	env2 := map[string]string{}
	// explicitly copy so that we're not editing another structure somewhere
	for k, v := range env {
		env2[k] = v
	}
	s := Session{env: env2, bramble: b}

	// if wd="" filepath.Abs will get the absolute path to the current working
	// directory
	var err error
	s.currentDirectory, err = filepath.Abs(wd)
	return s, err
}

func (s Session) expand(in string) string {
	return os.Expand(in, s.getEnv)
}

func (s Session) getEnv(key string) string {
	return s.env[key]
}

func copyM(src map[string]string, dst map[string]string) {
	for k, v := range src {
		dst[k] = v // copy all data from the current object to the new one
	}
}

func (s Session) copy() Session {
	out := Session{
		currentDirectory: s.currentDirectory,
		bramble:          s.bramble,
		env:              map[string]string{},
	}
	copyM(s.env, out.env)
	return out
}

func (s Session) setEnv(key, value string) Session {
	copy := s.copy()
	copy.env[key] = value
	return copy
}

func (s Session) envArray() (out []string) {
	for k, v := range s.env {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(out)
	return
}

func (s Session) cd(path string) (out string, err error) {
	if filepath.IsAbs(path) {
		out = path
	} else {
		out = filepath.Join(s.currentDirectory, path)
	}
	_, err = os.Stat(out)
	if err != nil {
		errors.Wrap(err, fmt.Sprintf("can't change directory to %q", out))
	}
	return
}

var (
	_ starlark.Value    = new(Session)
	_ starlark.HasAttrs = new(Session)
)

func (s Session) String() string        { return "<session>" }
func (s Session) Freeze()               { s.frozen = true }
func (s Session) Type() string          { return "session" }
func (s Session) Truth() starlark.Bool  { return starlark.True }
func (s Session) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("session") }

func (s Session) AttrNames() []string {
	return []string{
		"cd",
		"environ",
		"expand",
		"getenv",
		"setenv",
		"wd",
		"cmd",
	}
}

func (s Session) newCmdFunction() CmdFunction {
	return CmdFunction{
		bramble: s.bramble,
		session: s,
	}
}

func (s Session) Attr(name string) (val starlark.Value, err error) {
	callables := map[string]starutil.CallableFunc{
		"cd":     s.cdFn,
		"expand": s.expandFn,
		"setenv": s.setenvFn,
		"getenv": s.getenvFn,
		"cmd":    s.newCmdFunction().CallInternal,
	}
	if fn, ok := callables[name]; ok {
		return starutil.Callable{ThisName: name, ParentName: "session", Callable: fn}, nil
	}
	switch name {
	case "environ":
		out := starlark.NewDict(len(s.env))
		// TODO: cache this?
		for k, v := range s.env {
			// errors will not be outputted for a .SetKey that is not frozen, not
			// iterating, and just being passed string
			_ = out.SetKey(starlark.String(k), starlark.String(v))
		}
		return out, nil
	case "wd":
		return starlark.String(s.currentDirectory), nil
	}
	return nil, nil
}

func (s Session) expandFn(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var value starlark.String
	if err = starlark.UnpackArgs("expand", args, kwargs, "value", &value); err != nil {
		return
	}
	return starlark.String(s.expand(value.GoString())), nil
}
func (s Session) cdFn(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var path starlark.String
	if err = starlark.UnpackArgs("cd", args, kwargs, "path", &path); err != nil {
		return
	}
	// Does not edit Session.
	s.currentDirectory, err = s.cd(path.GoString())
	return s, err
}

func (s Session) getenvFn(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var key starlark.String
	if err = starlark.UnpackArgs("getenv", args, kwargs, "key", &key); err != nil {
		return
	}
	return starlark.String(s.getEnv(key.GoString())), nil
}

func (s Session) setenvFn(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var key starlark.String
	var value starlark.String
	if err = starlark.UnpackArgs("setenv", args, kwargs, "key", &key, "value", &value); err != nil {
		return
	}
	return s.setEnv(
		key.GoString(),
		value.GoString(),
	), nil
}
