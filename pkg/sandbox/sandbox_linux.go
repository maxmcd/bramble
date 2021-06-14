// +build linux

package sandbox

import (
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/creack/pty"
	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/sys/unix"
)

const (
	newNamespaceStepArg = "newNamespace"
	setupStepArg        = "setup"
	setUIDExecName      = "bramble-setuid"
)

func firstArgMatchesStep() bool {
	switch os.Args[0] {
	case newNamespaceStepArg, setupStepArg, execStepArg:
		return true
	}
	return false
}

func entrypoint() (err error) {
	if len(os.Args) <= 1 {
		return errors.New("unexpected argument count for sandbox step")
	}
	s, err := parseSerializedArg(os.Args[1])
	if err != nil {
		return err
	}
	switch os.Args[0] {
	case newNamespaceStepArg:
		return s.newNamespaceStep()
	case setupStepArg:
		return s.setupStep()
	case execStepArg:
		s.runExecStep()
		return nil
	default:
		return errors.New("first argument didn't match any known sandbox steps")
	}
}

func (s Sandbox) runCommand() (*exec.Cmd, error) {
	serialized, err := s.serializeArg()
	if err != nil {
		return nil, err
	}
	// TODO: allow reference to self
	// TODO: figure out what ^ means
	path, err := exec.LookPath(setUIDExecName)
	if err != nil {
		return nil, err
	}
	logger.Debugw("newSanbox", "execpath", path)
	// interrupt will be caught be the child process and the process
	// will exiting, causing this process to exit
	return &exec.Cmd{
		Path:   path,
		Args:   []string{newNamespaceStepArg, serialized},
		Stdin:  s.Stdin,
		Stdout: s.Stdout,
		Stderr: s.Stderr,
	}, nil
}

func (s Sandbox) newNamespaceStep() (err error) {
	selfExe, err := os.Readlink("/proc/self/exe")
	if err != nil {
		return err
	}
	defer func() {
		logger.Debugw("clean up chrootDir", "path", s.ChrootPath)
		if er := os.RemoveAll(s.ChrootPath); er != nil {
			logger.Debugw("error cleaning up", "err", er)
			if err == nil {
				err = errors.Wrap(er, "error removing all files in "+s.ChrootPath)
			}
		}
	}()
	serialized, err := s.serializeArg()
	if err != nil {
		return err
	}

	var cloneFlags uintptr = syscall.CLONE_NEWUTS |
		syscall.CLONE_NEWNS |
		syscall.CLONE_NEWPID

	if s.DisableNetwork {
		cloneFlags |= syscall.CLONE_NEWNET
	}

	// interrupt will be caught be the child process and the process
	// will exiting, causing this process to exit
	ignoreInterrupt()

	cmd := &exec.Cmd{
		Path: selfExe,
		Args: []string{setupStepArg, serialized},
		SysProcAttr: &syscall.SysProcAttr{
			// maybe sigint will allow the child more time to clean up its mounts????
			Pdeathsig:  unix.SIGINT,
			Cloneflags: cloneFlags,
		},
	}

	// We must use a pty here to enable interactive input. If we naively pass
	// os.Stdin to an exec.Cmd then we run into issues with the parent and
	// child terminals getting confused about who is supposed to process various
	// control signals.
	// We can then just set to raw and copy the bytes across. We could remove
	// the pty entirely for jobs that don't pass a terminal as a stdin.
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return errors.Wrap(err, "error starting pty")
	}
	defer func() { _ = ptmx.Close() }()

	// only handle stdin and set raw if it's an interactive terminal
	if os.Stdin != nil && terminal.IsTerminal(int(os.Stdin.Fd())) {
		// Handle pty resize
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGWINCH)
		go func() {
			for range ch {
				if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
					log.Printf("error resizing pty: %s", err)
				}
			}
		}()
		ch <- syscall.SIGWINCH // Initial resize.
		oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return err
		}
		// restore when complete
		defer func() { _ = terminal.Restore(int(os.Stdin.Fd()), oldState) }()
		go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	}
	_, _ = io.Copy(os.Stdout, ptmx)
	return errors.Wrap(cmd.Wait(), "error running newNamespace")
}

func (s Sandbox) setupStep() (err error) {
	logger.Debugw("setup chroot", "dir", s.ChrootPath)
	creds := &syscall.Credential{
		Gid: uint32(s.GroupID),
		Uid: uint32(s.UserID),
	}
	if err := os.Chown(s.ChrootPath, int(creds.Uid), int(creds.Gid)); err != nil {
		return err
	}

	chr := newChroot(s.ChrootPath, s.Mounts)
	defer func() {
		if er := chr.Cleanup(); er != nil {
			if err == nil {
				err = er
			} else {
				logger.Debugw("error during cleanup", "err", er)
			}
		}
	}()
	var selfExe string
	{
		// hardlink in executable
		selfExe, err = os.Readlink("/proc/self/exe")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(s.ChrootPath, filepath.Dir(selfExe)), 0777); err != nil {
			return err
		}
		if err = os.Link(selfExe, filepath.Join(s.ChrootPath, selfExe)); err != nil {
			return err
		}
	}

	if err := chr.Init(); err != nil {
		return err
	}

	serialized, err := s.serializeArg()
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(interruptContext(), selfExe)
	cmd.Path = selfExe
	cmd.Args = []string{execStepArg, serialized}
	cmd.Env = append([]string{"USER=bramblebuild0", "HOME=/homeless"}, s.Env...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: creds,
	}
	return cmd.Run()
}
