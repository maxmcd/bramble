package main

import (
	"context"
	"os"

	"github.com/maxmcd/bramble/pkg/sandbox"
)

func main() {
	sandbox.Entrypoint()

	sandbox := sandbox.Sandbox{
		Stdin:  os.Stdin,
		Stderr: os.Stderr,
		Stdout: os.Stdout,
	}
	sandbox.Run(context.Background())
}

// import (
// 	"fmt"
// 	"io/ioutil"
// 	"log"
// 	"os"
// 	"os/user"
// 	"path/filepath"
// 	"runtime"
// 	"strconv"
// 	"testing/fstest"

// 	"github.com/opencontainers/runc/libcontainer"
// 	"github.com/opencontainers/runc/libcontainer/configs"
// 	"github.com/opencontainers/runc/libcontainer/devices"
// 	_ "github.com/opencontainers/runc/libcontainer/nsenter"
// 	"github.com/opencontainers/runc/libcontainer/specconv"
// 	"golang.org/x/sys/unix"
// )

// func contConfig() *configs.Config {

// 	resolvConfBytes, err := os.ReadFile("/etc/resolv.conf")
// 	if err != nil {
// 		panic(err)
// 	}
// 	files["etc/resolv.conf"] = &fstest.MapFile{
// 		Data: resolvConfBytes,
// 		Mode: 0644,
// 	}
// 	chrootDir, err := ioutil.TempDir("", "bramble-chroot-")
// 	if err != nil {
// 		panic(err)
// 	}

// 	rootfsPath, err := filepath.Abs("./rootfs")
// 	if err != nil {
// 		panic(err)
// 	}
// 	_, _ = chrootDir, rootfsPath

// 	for path, file := range files {
// 		loc := filepath.Join(chrootDir, path)
// 		if err := os.MkdirAll(filepath.Dir(loc), 0777); err != nil {
// 			panic(err)
// 		}
// 		f, err := os.OpenFile(filepath.Join(chrootDir, path), os.O_CREATE|os.O_RDWR|os.O_TRUNC, file.Mode)
// 		if err != nil {
// 			panic(err)
// 		}
// 		_, _ = f.Write(file.Data)
// 		if err := f.Close(); err != nil {
// 			panic(err)
// 		}
// 	}

// 	u, err := user.Current()
// 	if err != nil {
// 		panic(err)
// 	}
// 	uid, _ := strconv.Atoi(u.Uid)
// 	gid, _ := strconv.Atoi(u.Gid)

// 	defaultMountFlags := unix.MS_NOEXEC | unix.MS_NOSUID | unix.MS_NODEV
// 	var devices []*devices.Rule
// 	for _, device := range specconv.AllowedDevices {
// 		devices = append(devices, &device.Rule)
// 	}
// 	caps := []string{
// 		"CAP_AUDIT_WRITE",
// 		"CAP_KILL",
// 		"CAP_NET_BIND_SERVICE",
// 	}
// 	return
// }

// func main() {
// 	if len(os.Args) > 1 && os.Args[1] == "init" {
// 		runtime.GOMAXPROCS(1)
// 		runtime.LockOSThread()
// 		factory, err := libcontainer.New("")
// 		if err != nil {
// 			log.Fatal(err)
// 		}
// 		if err := factory.StartInitialization(); err != nil {
// 			log.Fatal(err)
// 		}
// 		panic("unreachable")
// 	}

// 	factory, err := libcontainer.New("/tmp/runc",
// 		libcontainer.RootlessCgroupfs,
// 		libcontainer.InitArgs(os.Args[0], "init"))
// 	if err != nil {
// 		panic(err)
// 	}

// 	container, err := factory.Create("fortesting", contConfig())
// 	if err != nil {
// 		panic(err)
// 	}
// 	process := &libcontainer.Process{
// 		Args: []string{"sh"},
// 		Env: []string{
// 			"PATH=/home/maxm/bramble/bramble_store_padding/bramble_/2xqocntumrj4vp6buucoma2q6a6dfvmf/bin/",
// 			// "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
// 			"TERM=xterm",
// 		},
// 		User:     "root",
// 		Stdin:    os.Stdin,
// 		Stdout:   os.Stdout,
// 		Stderr:   os.Stderr,
// 		Init:     true,
// 		Cwd:      "/",
// 		LogLevel: "debug",
// 	}
// 	defer container.Destroy()
// 	fmt.Println("running")
// 	if err := container.Run(process); err != nil {
// 		_ = container.Destroy()
// 		panic(err)
// 		return
// 	}

// 	fmt.Println("starting wait")

// 	// wait for the process to finish.
// 	if _, err := process.Wait(); err != nil {
// 		_ = container.Destroy()
// 		panic(err)
// 	}

// 	fmt.Println("past wait")

// 	// destroy the container.
// 	container.Destroy()
// }
