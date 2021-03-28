// +build darwin
package sandbox

import (
	"os/exec"
)

func (s Sandbox) runCommand() (*exec.Cmd, error) {
	return exec.Command("sandbox-exec"), nil
}

func firstArgMatchesStep() bool {
	return false
}
func entrypoint() error {
	return nil
}
