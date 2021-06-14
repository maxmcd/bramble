// +build darwin

package sandbox

import (
	"os/exec"
)

func (s Sandbox) runCommand() (*exec.Cmd, error) {
	profile := `(version 1)
	(deny default)
	(allow process-exec*)
	(import "/System/Library/Sandbox/Profiles/bsd.sb")`
	if !s.DisableNetwork {
		profile += `(allow network*)`
	}
	cmd := exec.Command("sandbox-exec", "-p", profile, s.Path)
	cmd.Args = append(cmd.Args, s.Args...)
	cmd.Dir = s.Dir
	cmd.Env = s.Env
	cmd.Stderr = s.Stderr
	cmd.Stdout = s.Stdout
	cmd.Stdin = s.Stdin
	return cmd, nil
}

func firstArgMatchesStep() bool {
	return false
}
func entrypoint() error {
	return nil
}
