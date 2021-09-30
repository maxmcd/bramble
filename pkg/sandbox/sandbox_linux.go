package sandbox

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing/fstest"

	"github.com/moby/term"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/opencontainers/runc/libcontainer/devices"
	"github.com/opencontainers/runc/libcontainer/utils"

	// Needed or libcontainer entrypoint call won't work
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
	"github.com/opencontainers/runc/libcontainer/specconv"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

func init() {
	entrypoint = func() {
		// Libcontainer will take the "init" are we pass as the fake path and
		// prepend the current working directory. So just check if it ends in
		// the name we need.
		if !(len(os.Args) > 1 && strings.HasSuffix(os.Args[0], initArg) && os.Args[1] == initArg) {
			return
		}
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()
		factory, err := libcontainer.New("")
		if err != nil {
			panic(err)
		}
		if err := factory.StartInitialization(); err != nil {
			panic(err)
		}
		panic("unreachable")
	}
}

type container struct {
	container libcontainer.Container
	tmpdir    string
	sandbox   Sandbox
	process   *libcontainer.Process
}

func initRootfs(path string) (err error) {
	files := minimalRootFS()

	// Copy resolv.conf settings into the container
	resolvConfBytes, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return errors.Wrap(err, "error attempting to read /etc/resolv.conf")
	}
	files["etc/resolv.conf"] = &fstest.MapFile{
		Data: resolvConfBytes,
		Mode: 0644,
	}
	_ = os.MkdirAll(filepath.Join(path, "tmp"), 0755)
	for loc, file := range files {
		loc = filepath.Join(path, loc)
		if err := os.MkdirAll(filepath.Dir(loc), 0755); err != nil {
			return err
		}
		f, err := os.OpenFile(loc, os.O_CREATE|os.O_RDWR|os.O_TRUNC, file.Mode)
		if err != nil {
			return err
		}
		_, _ = f.Write(file.Data)
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

func newContainer(s Sandbox) (c container, err error) {
	c.sandbox = s
	cfg := defaultRootlessConfig()
	uid, gid, err := userAndGroupIDs()
	if err != nil {
		err = errors.Wrap(err, "error attempting to read current users uid and gid")
		return
	}

	cfg.MaskPaths = append(cfg.MaskPaths, s.HiddenPaths...)
	cfg.ReadonlyPaths = append(cfg.ReadonlyPaths, s.ReadOnlyPaths...)

	cfg.UidMappings[0].HostID = uid
	cfg.GidMappings[0].HostID = gid
	if !s.Network {
		// NEWNET creates a new network namespace, it won't have a working network.
		cfg.Namespaces = append(cfg.Namespaces, configs.Namespace{Type: configs.NEWNET})
	}
	for _, mount := range s.Mounts {
		src, ro, valid := parseMount(mount)
		if !valid {
			return c, errors.Errorf("mount %q is incorrectly formatted", mount)
		}
		flags := unix.MS_BIND | unix.MS_NOSUID | unix.MS_NODEV | unix.MS_REC
		if ro {
			flags |= unix.MS_RDONLY
		}
		cfg.Mounts = append(cfg.Mounts, &configs.Mount{
			Source:      src,
			Device:      "bind",
			Destination: src,
			Flags:       flags,
		})
	}

	c.tmpdir, err = ioutil.TempDir("", "bramble-chroot-")
	if err != nil {
		return
	}

	stateDir := filepath.Join(c.tmpdir, "/state")
	rootFSDir := filepath.Join(c.tmpdir, "/rootfs")
	for _, dir := range []string{stateDir, rootFSDir} {
		if err = os.Mkdir(dir, 0744); err != nil {
			return
		}
	}
	if err = initRootfs(rootFSDir); err != nil {
		err = errors.Wrap(err, "error writing files to rootfs")
		return
	}
	cfg.Rootfs = rootFSDir

	factory, err := libcontainer.New(stateDir,
		libcontainer.RootlessCgroupfs,
		libcontainer.InitArgs(initArg, initArg))
	if err != nil {
		err = errors.Wrap(err, "error initializing container factory")
		return
	}
	c.container, err = factory.Create("bramble", cfg)
	if err != nil {
		err = errors.Wrap(err, "error creating container")
	}

	return
}

func userAndGroupIDs() (uid, gid int, err error) {
	u, err := user.Current()
	if err != nil {
		return
	}
	if uid, err = strconv.Atoi(u.Uid); err != nil {
		return
	}
	gid, err = strconv.Atoi(u.Gid)
	return
}

func terminate(p *libcontainer.Process) {
	_ = p.Signal(unix.SIGKILL)
	_, _ = p.Wait()
}

func (c *container) Run() (err error) {
	if c.process != nil {
		return errors.New("Run command has already been called")
	}

	c.process = &libcontainer.Process{
		Args:   c.sandbox.Args,
		Env:    c.sandbox.Env,
		User:   "root",
		Stdin:  c.sandbox.Stdin,
		Stdout: c.sandbox.Stdout,
		Stderr: c.sandbox.Stderr,
		Init:   true,
		Cwd:    c.sandbox.Dir,
	}

	var t *tty
	// If we are using a real terminal then spawn a tty
	if stdinF, ok := c.sandbox.Stdin.(*os.File); ok && term.IsTerminal(stdinF.Fd()) {
		t := &tty{}
		if err := t.initHostConsole(); err != nil {
			return err
		}
		parent, child, err := utils.NewSockPair("console")
		if err != nil {
			return err
		}
		c.process.ConsoleSocket = child
		t.postStart = append(t.postStart, parent, child)
		t.consoleC = make(chan error, 1)
		go func() {
			t.consoleC <- t.recvtty(c.process, parent)
		}()
		defer t.Close()
	}
	defer func() { _ = c.Destroy() }()
	if err := c.container.Run(c.process); err != nil {
		return err
	}
	if t != nil {
		if err = t.waitConsole(); err != nil {
			terminate(c.process)
			return err
		}
		if err = t.ClosePostStart(); err != nil {
			terminate(c.process)
			return err
		}
		handler := newSignalHandler()
		status, err := handler.forward(c.process, t, false)
		if err != nil {
			terminate(c.process)
			return err
		}
		if status != 0 {
			return ExitError{ExitCode: status}
		}
	} else {
		state, err := c.process.Wait()
		if err != nil {
			return err
		}
		if state.ExitCode() != 0 {
			return errors.Errorf("Process exited with non-zero exit code %d but we make the message funny because we're not sure if this is handled above", state.ExitCode())
		}
	}
	return
}

func (c *container) Stop() (err error) {
	if c.process != nil {
		return c.process.Signal(syscall.SIGKILL)
	}
	return nil
}

func combineErrors(errs ...error) (err error) {
	var ers []error
	for _, e := range errs {
		if e != nil {
			ers = append(ers, e)
		}
	}
	switch len(ers) {
	case 1:
		return ers[0]
	case 0:
		return nil
	default:
		return errors.Errorf("got errors %q", ers)
	}
}

func (c *container) Destroy() (err error) {
	return combineErrors(os.RemoveAll(c.tmpdir), c.container.Destroy())
}

func configDevices() (devices []*devices.Rule) {
	for _, device := range specconv.AllowedDevices {
		devices = append(devices, &device.Rule)
	}
	return devices
}

func minimalRootFS() fstest.MapFS {
	return fstest.MapFS{
		"etc/passwd": &fstest.MapFile{
			Data: []byte(`root:x:0:0:root:/root:/bin/sh
daemon:x:1:1:daemon:/usr/sbin:/bin/false
bin:x:2:2:bin:/bin:/bin/false
sys:x:3:3:sys:/dev:/bin/false
sync:x:4:100:sync:/bin:/bin/sync
mail:x:8:8:mail:/var/spool/mail:/bin/false
www-data:x:33:33:www-data:/var/www:/bin/false
operator:x:37:37:Operator:/var:/bin/false
nobody:x:65534:65534:nobody:/home:/bin/false
`),
			Mode: 0644,
		},
		"etc/group": &fstest.MapFile{
			Data: []byte(`root:x:0:
daemon:x:1:
bin:x:2:
sys:x:3:
adm:x:4:
tty:x:5:
disk:x:6:
lp:x:7:
mail:x:8:
kmem:x:9:
wheel:x:10:root
cdrom:x:11:
dialout:x:18:
floppy:x:19:
video:x:28:
audio:x:29:
tape:x:32:
www-data:x:33:
operator:x:37:
utmp:x:43:
plugdev:x:46:
staff:x:50:
lock:x:54:
netdev:x:82:
users:x:100:
nobody:x:65534:
`),
			Mode: 0644,
		},
	}
}

func defaultRootlessConfig() *configs.Config {
	defaultMountFlags := unix.MS_NOEXEC | unix.MS_NOSUID | unix.MS_NODEV
	caps := []string{
		"CAP_AUDIT_WRITE",
		"CAP_KILL",
		"CAP_NET_BIND_SERVICE",
	}
	return &configs.Config{
		Capabilities: &configs.Capabilities{
			Bounding:    caps,
			Effective:   caps,
			Inheritable: caps,
			Permitted:   caps,
			Ambient:     caps,
		},
		Rlimits: []configs.Rlimit{
			{
				Type: unix.RLIMIT_NOFILE,
				Hard: uint64(1025),
				Soft: uint64(1025),
			},
		},
		RootlessEUID:    true,
		RootlessCgroups: true,
		Cgroups: &configs.Cgroup{
			Name:   "bramble",
			Parent: "system",
			Resources: &configs.Resources{
				MemorySwappiness: nil,
				Devices:          configDevices(),
			},
		},
		Devices:         specconv.AllowedDevices,
		NoNewPrivileges: true,

		// https://github.com/opencontainers/runc/issues/1456#issuecomment-303784735
		NoNewKeyring: true,
		NoPivotRoot:  true,

		// Rootfs:          chrootDir,
		Readonlyfs: false,
		Hostname:   "runc",
		Mounts: []*configs.Mount{
			{
				Source:      "proc",
				Destination: "/proc",
				Device:      "proc",
				Flags:       defaultMountFlags,
			},
			{
				Source:      "tmpfs",
				Destination: "/dev",
				Device:      "tmpfs",
				Flags:       unix.MS_NOSUID | unix.MS_STRICTATIME,
				Data:        "mode=755,size=65536k",
			},
			{
				Source:      "devpts",
				Destination: "/dev/pts",
				Device:      "devpts",
				Flags:       unix.MS_NOSUID | unix.MS_NOEXEC,
				Data:        "newinstance,ptmxmode=0666,mode=0620",
			},
			{
				Device:      "tmpfs",
				Source:      "shm",
				Destination: "/dev/shm",
				Data:        "mode=1777,size=65536k",
				Flags:       defaultMountFlags,
			},
			{
				Source:      "mqueue",
				Destination: "/dev/mqueue",
				Device:      "mqueue",
				Flags:       defaultMountFlags,
			},
			{
				Source:      "/sys",
				Device:      "bind",
				Destination: "/sys",
				Flags:       defaultMountFlags | unix.MS_RDONLY | unix.MS_BIND | unix.MS_REC,
			},
		},
		Namespaces: configs.Namespaces([]configs.Namespace{
			{Type: configs.NEWNS},
			{Type: configs.NEWPID},
			{Type: configs.NEWIPC},
			{Type: configs.NEWUTS},
			{Type: configs.NEWUSER},
			{Type: configs.NEWCGROUP},
		}),
		UidMappings: []configs.IDMap{
			{
				ContainerID: 0,
				// HostID:      0,
				Size: 1,
			},
		},
		GidMappings: []configs.IDMap{
			{
				ContainerID: 0,
				// HostID:      0,
				Size: 1,
			},
		},
		MaskPaths: []string{
			"/proc/acpi",
			"/proc/asound",
			"/proc/kcore",
			"/proc/keys",
			"/proc/latency_stats",
			"/proc/timer_list",
			"/proc/timer_stats",
			"/proc/sched_debug",
			"/sys/firmware",
			"/proc/scsi",
		},
		ReadonlyPaths: []string{
			"/proc/bus",
			"/proc/fs",
			"/proc/irq",
			"/proc/sys",
			"/proc/sysrq-trigger",
		},
	}
}
