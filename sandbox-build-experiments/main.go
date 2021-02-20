package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/maxmcd/bramble/pkg/bramblepb"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
)

func userToCreds(u *user.User) *syscall.Credential {
	uid, _ := strconv.Atoi(u.Uid)
	guid, _ := strconv.Atoi(u.Gid)
	return &syscall.Credential{
		Gid: uint32(guid),
		Uid: uint32(uid),
	}
}

func run() (err error) {
	switch os.Args[0] {
	case "child":
		return Child()
	case "app":
		return App()
	default:
		return RootButNewNamespace()
	}
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func RootButNewNamespace() (err error) {
	conn, err := grpc.Dial(
		os.Args[1],
		grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}))
	if err != nil {
		return errors.Wrap(err, "grpc.Dial")
	}
	client := bramblepb.NewSingleBuilderClient(conn)
	clientInst, err := client.FetchBuild(context.Background())
	if err != nil {
		return errors.Wrap(err, "client.FetchBuild")
	}

	input, err := clientInst.Recv()
	if err != nil {
		return errors.Wrap(err, "inst.Recv")
	}
	spew.Dump(input)
	chrootDir, err := ioutil.TempDir("", "bramble-")
	if err != nil {
		return err
	}

	cmd := &exec.Cmd{
		Path: "/proc/self/exe",
		Args: []string{"child", chrootDir},
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: unix.SIGTERM,
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNET, // no network access
	}
	fmt.Println("running application")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error running the /bin/sh command - %s\n", err)
		os.Exit(1)
	}
	_ = os.RemoveAll(chrootDir)
	return nil
}

func Child() (err error) {
	buildUser, err := user.Lookup("bramblebuild0")
	if err != nil {
		log.Fatal(err)
	}
	chrootDir := os.Args[1]

	creds := userToCreds(buildUser)

	if err := os.Chown(chrootDir, int(creds.Uid), int(creds.Gid)); err != nil {
		return err
	}
	if err := os.Mkdir(chrootDir+"/proc", 0777); err != nil {
		return err
	}
	if err := os.Mkdir(chrootDir+"/bin", 0777); err != nil {
		return err
	}
	selfExe, err := os.Readlink("/proc/self/exe")
	if err != nil {
		return err
	}
	_ = os.Link(selfExe, chrootDir+"/bin/sbx")

	if err := syscall.Chroot(chrootDir); err != nil {
		return errors.Wrap(err, "chroot")
	}

	if err := os.Chdir("/"); err != nil {
		return errors.Wrap(err, "chdir")
	}
	if err := syscall.Mount("proc", "proc", "proc", 0, ""); err != nil {
		return errors.Wrap(err, "proc mount")
	}

	dirs, _ := ioutil.ReadDir("/bin")
	for _, dir := range dirs {
		fmt.Println(dir.Name())
	}

	cmd := &exec.Cmd{
		Path: "/bin/sbx",
		Args: []string{"app"},
	}
	cmd.Env = []string{"USER=bramblebuild0", "HOME=/homeless"}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: creds,
	}
	fmt.Println("running child")
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "child run app")
	}
	fmt.Println("cleaning up")
	if err := syscall.Unmount("proc", 0); err != nil {
		return err
	}
	if err := os.RemoveAll(chrootDir); err != nil {
		return err
	}
	return nil
}

func App() (err error) {
	fmt.Println("hello!")

	_, err = net.Dial("tcp", "google.com:80")
	spew.Dump(err)
	spew.Dump(user.Current())
	return nil
}
