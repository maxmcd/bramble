package bramble

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/maxmcd/bramble/pkg/starutil"
	"go.starlark.net/starlark"
)

type Session struct {
	env              map[string]string
	currentDirectory string
	frozen           bool
}

func newSession(wd string, env map[string]string) (*Session, error) {
	env2 := map[string]string{}
	if env == nil {
		// TODO: consider not allowing any external environment variables
		for _, kv := range os.Environ() {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) > 1 {
				env2[parts[0]] = parts[1]
			} else {
				env2[parts[0]] = ""
			}
		}
	} else {
		// explicitly copy so that we're not editing another structure somewhere
		for k, v := range env {
			env2[k] = v
		}
	}
	s := Session{env: env2}
	var err error
	// if wd="" filepath.Abs will get the absolute path to the current working
	// directory
	s.currentDirectory, err = filepath.Abs(wd)
	return &s, err
}

func (s *Session) expand(in string) string {
	return os.Expand(in, s.getEnv)
}

func (s *Session) getEnv(key string) string {
	return s.env[s.expand(key)]
}
func (s *Session) setEnv(key, value string) {
	s.env[s.expand(key)] = s.expand(value)
}

func (s *Session) envArray() (out []string) {
	for k, v := range s.env {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(out)
	return
}

func (s *Session) cd(path string) (err error) {
	if strings.HasPrefix(path, "/") {
		s.currentDirectory = path
	} else {
		s.currentDirectory = filepath.Join(s.currentDirectory, path)
	}
	_, err = os.Stat(s.currentDirectory)
	return
}

var (
	_ starlark.Value    = new(Session)
	_ starlark.HasAttrs = new(Session)
)

func (s *Session) String() string        { return "<session>" }
func (s *Session) Freeze()               { s.frozen = true }
func (s *Session) Type() string          { return "session" }
func (s *Session) Truth() starlark.Bool  { return starlark.True }
func (s *Session) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("session") }

func (s *Session) AttrNames() []string {
	return []string{
		"cd",
		"environ",
		"expand",
		"getenv",
		"setenv",
		"wd",
	}
}

func (s *Session) Attr(name string) (val starlark.Value, err error) {
	switch name {
	case "cd":
		return starutil.Callable{ThisName: "cd", ParentName: "session", Callable: s.cdFn}, nil
	case "environ":
		out := starlark.NewDict(len(s.env))
		// TODO: cache this?
		for k, v := range s.env {
			// errors will not be outputted for a .SetKey that is not frozen, not
			// iterating, and just being passed string
			_ = out.SetKey(starlark.String(k), starlark.String(v))
		}
		return out, nil
	case "expand":
		return starutil.Callable{ThisName: "expand", ParentName: "session", Callable: s.expandFn}, nil
	case "setenv":
		return starutil.Callable{ThisName: "setenv", ParentName: "session", Callable: s.setenvFn}, nil
	case "getenv":
		return starutil.Callable{ThisName: "getenv", ParentName: "session", Callable: s.getenvFn}, nil
	case "wd":
		return starlark.String(s.currentDirectory), nil
	}
	return nil, nil
}

func (s *Session) expandFn(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var value starlark.String
	if err = starlark.UnpackArgs("expand", args, kwargs, "value", &value); err != nil {
		return
	}
	return starlark.String(s.expand(value.GoString())), nil
}
func (s *Session) cdFn(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var path starlark.String
	if err = starlark.UnpackArgs("cd", args, kwargs, "path", &path); err != nil {
		return
	}
	return starlark.None, s.cd(path.GoString())
}

func (s *Session) getenvFn(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var key starlark.String
	if err = starlark.UnpackArgs("getenv", args, kwargs, "key", &key); err != nil {
		return
	}
	return starlark.String(s.getEnv(key.GoString())), nil
}

func (s *Session) setenvFn(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var key starlark.String
	var value starlark.String
	if err = starlark.UnpackArgs("setenv", args, kwargs, "key", &key, "value", &value); err != nil {
		return
	}
	s.setEnv(
		key.GoString(),
		value.GoString(),
	)
	return starlark.None, nil
}
