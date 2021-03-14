package sandbox

import "os/exec"

// +build darwin

func runExecPath() (path string, err error) {
	return exec.LookPath("sandbox-exec")
}

func runFirstArgs(serialized string) []string {
	return []string{newNamespaceStepArg, serialized}
}
