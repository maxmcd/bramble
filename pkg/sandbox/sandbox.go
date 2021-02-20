package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/maxmcd/bramble/pkg/store"
	"github.com/maxmcd/gosh/shell"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

const (
	newNamespaceStepArg = "newNamespace"
	setupStepArg        = "setup"
	execStepArg         = "exec"
	setUIDExecName      = "bramble-setuid"
	debugMagicString    = "debugmagicstring"
)

func RunSetUID() (err error) {
	firstArg := ""
	if len(os.Args) > 1 {
		firstArg = os.Args[1]
	}
	b, chrootDir, err := parseSerializedArg(firstArg)
	switch os.Args[0] {
	case newNamespaceStepArg, setupStepArg, execStepArg:
		if err != nil {
			return err
		}
	}
	switch os.Args[0] {
	case newNamespaceStepArg:
		return b.newNamespaceStep()
	case setupStepArg:
		return b.setupStep(chrootDir)
	case execStepArg:
		b.runExecStep()
		return nil
	default:
		return errors.New("can't run process without specific path argument")
	}
}

func RunDebug() (err error) {
	store, err := store.NewStore()
	if err != nil {
		return err
	}
	sbx := &Sandbox{
		Store:  store,
		Path:   debugMagicString,
		Stdin:  os.Stdin,
		Stderr: os.Stderr,
		Stdout: os.Stdout,
	}
	return sbx.Run()
}

type Sandbox struct {
	Store  store.Store
	Stdin  io.Reader `json:"-"`
	Stdout io.Writer `json:"-"`
	Stderr io.Writer `json:"-"`
	Path   string
	Args   []string
	Dir    string
}
type serializedSandbox struct {
	Sandbox
	ChrootDir string
}

func (s Sandbox) serializeArg(chrootDir string) (string, error) {
	sb := serializedSandbox{
		Sandbox:   s,
		ChrootDir: chrootDir,
	}
	byt, err := json.Marshal(sb)

	return string(byt), err
}

func parseSerializedArg(arg string) (s Sandbox, chrootDir string, err error) {
	var sb serializedSandbox
	if err = json.Unmarshal([]byte(arg), &sb); err != nil {
		return
	}
	return sb.Sandbox, sb.ChrootDir, nil
}

func (s Sandbox) Run() (err error) {
	if s.Store.IsEmpty() {
		return errors.New("sandbox has an empty store")
	}
	serialized, err := s.serializeArg("")
	if err != nil {
		return err
	}
	path, err := exec.LookPath(setUIDExecName)
	if err != nil {
		return err
	}
	logger.Debugw("newSanbox", "execpath", path)
	// TODO: audit the contents of serialized
	cmd := &exec.Cmd{
		Path: path,
		Args: []string{newNamespaceStepArg, serialized},
	}
	cmd.Stdin = s.Stdin
	cmd.Stdout = s.Stdout
	cmd.Stderr = s.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running newSandbox - %w", err)
	}
	return nil
}

func (s Sandbox) newNamespaceStep() (err error) {
	selfExe, err := os.Readlink("/proc/self/exe")
	if err != nil {
		return err
	}
	chrootDir, err := s.Store.TempDir()
	if err != nil {
		return err
	}
	defer func() {
		logger.Debugw("clean up chrootDir", "path", chrootDir)
		if er := os.RemoveAll(chrootDir); er != nil && err == nil {
			err = errors.Wrap(er, "error removing all files in "+chrootDir)
		}
	}()
	serialized, err := s.serializeArg(chrootDir)
	if err != nil {
		return err
	}
	cmd := &exec.Cmd{
		Path:   selfExe,
		Args:   []string{setupStepArg, serialized},
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		SysProcAttr: &syscall.SysProcAttr{
			Pdeathsig: unix.SIGTERM,
			Cloneflags: syscall.CLONE_NEWUTS |
				syscall.CLONE_NEWNS |
				syscall.CLONE_NEWPID |
				syscall.CLONE_NEWNET, // no network access
		},
	}
	return errors.Wrap(cmd.Run(), "error running newNamespace")
}

func (s Sandbox) setupStep(chrootDir string) (err error) {
	logger.Debugw("setup chroot", "dir", chrootDir)
	buildUser, err := user.Lookup("bramblebuild0")
	if err != nil {
		return err
	}
	creds := userToCreds(buildUser)
	if err := os.Chown(chrootDir, int(creds.Uid), int(creds.Gid)); err != nil {
		return err
	}

	chr := newChroot(chrootDir, []string{
		s.Store.StorePath + ":ro",
		// filepath.Join(s.Store.StorePath, "within"),
	})
	defer func() {
		if er := chr.Cleanup(); er != nil && err == nil {
			err = er
		}
	}()
	var selfExe string
	{
		// hardlink in executable
		selfExe, err = os.Readlink("/proc/self/exe")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(chrootDir, filepath.Dir(selfExe)), 0777); err != nil {
			return err
		}
		if err = os.Link(selfExe, filepath.Join(chrootDir, selfExe)); err != nil {
			return err
		}
	}

	if err := chr.Init(); err != nil {
		return err
	}

	cmd := exec.CommandContext(interruptContext(), selfExe)
	cmd.Path = selfExe
	cmd.Args = []string{execStepArg}
	cmd.Env = []string{"USER=bramblebuild0", "HOME=/homeless"}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: creds,
	}
	return cmd.Run()
}

func interruptContext() context.Context {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c
		cancel()
	}()
	return ctx
}

func (s Sandbox) runExecStep() {
	fmt.Println(s)
	shell.Run()
}

func userToCreds(u *user.User) *syscall.Credential {
	uid, _ := strconv.Atoi(u.Uid)
	guid, _ := strconv.Atoi(u.Gid)
	return &syscall.Credential{
		Gid: uint32(guid),
		Uid: uint32(uid),
	}
}
