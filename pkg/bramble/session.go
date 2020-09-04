package bramble

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type session struct {
	env              map[string]string
	currentDirectory string
}

func newSession(wd string, env map[string]string) (*session, error) {
	env2 := map[string]string{}
	if env == nil {
		for _, kv := range os.Environ() {
			parts := strings.SplitN(kv, "=", 1)
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
	s := session{env: env2}
	var err error
	// if wd="" filepath.Abs will get the absolute path to the current working
	// directory
	s.currentDirectory, err = filepath.Abs(wd)
	return &s, err
}

func (s *session) expand(in string) string {
	return os.Expand(in, s.getEnv)
}

func (s *session) getEnv(key string) string {
	return s.env[s.expand(key)]
}
func (s *session) setEnv(key, value string) {
	s.env[s.expand(key)] = s.expand(value)
}

func (s *session) envArray() (out []string) {
	for k, v := range s.env {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(out)
	return
}

func (s *session) cd(path string) (err error) {
	if strings.HasPrefix(path, "/") {
		s.currentDirectory = path
	} else {
		s.currentDirectory = filepath.Join(s.currentDirectory, path)
	}
	_, err = os.Stat(s.currentDirectory)
	return
}
