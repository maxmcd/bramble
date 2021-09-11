package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/pkg/errors"
)

type chroot struct {
	initialized bool
	chrooted    bool
	location    string
	mounts      []string
}

func newChroot(location string, mounts []string) *chroot {
	return &chroot{
		location: location,
		mounts:   mounts,
	}
}

func makedev(major int, minor int) int {
	return (minor & 0xff) | (major&0xfff)<<8
}

func (chr *chroot) Init() (err error) {
	if chr.initialized {
		return errors.New("chroot env already initialized")
	}
	chr.initialized = true

	// reset file mode creation mask to zero
	syscall.Umask(0)

	for _, dir := range []string{"proc", "dev", "tmp"} {
		if err := os.Mkdir(filepath.Join(chr.location, dir), 0755); err != nil {
			return err
		}
	}
	if err = os.Chmod(filepath.Join(chr.location, "tmp"), 0777); err != nil {
		return errors.Wrap(err, "chmod tmp 0777")
	}

	if err := syscall.Mknod(filepath.Join(chr.location, "dev", "/null"),
		syscall.S_IFCHR|0666, makedev(1, 3)); err != nil {
		return errors.Wrap(err, "mknod /dev/null")
	}
	if err := syscall.Mknod(filepath.Join(chr.location, "dev", "/random"),
		syscall.S_IFCHR|0666, makedev(1, 8)); err != nil {
		return errors.Wrap(err, "mknod /dev/random")
	}
	if err := syscall.Mknod(filepath.Join(chr.location, "dev", "/urandom"),
		syscall.S_IFCHR|0666, makedev(1, 9)); err != nil {
		return errors.Wrap(err, "mknod /dev/urandom")
	}

	// mount proc
	if err := syscall.Mount("proc", filepath.Join(chr.location, "proc"), "proc", 0, ""); err != nil {
		return errors.Wrap(err, "proc mount")
	}
	for _, mount := range chr.mounts {
		src, ro, valid := parseMount(mount)
		if !valid {
			return fmt.Errorf("mount %q is incorrectly formatted, should be like /proc:/proc or /opt:/app:ro", mount)
		}
		srcfi, err := os.Stat(src)
		if err != nil {
			return errors.Wrap(err, "trying to read mount source")
		}
		targetDir := src
		if !srcfi.IsDir() {
			targetDir = filepath.Dir(src)
		}
		if err := os.MkdirAll(filepath.Join(chr.location, targetDir), 0755); err != nil {
			return errors.Wrap(err, "making destination directory")
		}
		target := filepath.Join(chr.location, src)
		if err := syscall.Mount(src, target, "bind", syscall.MS_BIND, ""); err != nil {
			return errors.Wrap(err, "error mounting location: "+src)
		}
		if err := syscall.Mount("none", target, "", syscall.MS_SHARED, ""); err != nil {
			return fmt.Errorf("could not make mount point %s: %w", src, err)
		}
		if ro {
			logger.Debugw("binding readonly")
			if err := syscall.Mount(src, target, "bind", uintptr(syscall.MS_BIND|syscall.MS_REMOUNT|syscall.MS_RDONLY), ""); err != nil {
				return errors.Wrap(err, "error remounting to readonly")
			}
		}
	}

	if err := syscall.Chroot(chr.location); err != nil {
		return errors.Wrap(err, "chroot")
	}
	chr.chrooted = true

	if err := os.Chdir("/"); err != nil {
		return errors.Wrap(err, "chdir")
	}
	return nil
}

func (chr *chroot) Cleanup() (err error) {
	if !chr.initialized {
		return nil
	}
	logger.Debugw("cleaning up env", "mounts", chr.mounts)

	root := "/"
	if !chr.chrooted {
		root = chr.location
	}
	if err := syscall.Unmount(filepath.Join(root, "proc"), 0); err != nil {
		return errors.Wrap(err, "error unmounting proc")
	}

	// must go in reverse order in case we have mounts within mounts
	for i := len(chr.mounts) - 1; i >= 0; i-- {
		mount := chr.mounts[i]
		loc, _, _ := parseMount(mount)
		loc = filepath.Join(root, loc)
		logger.Debugw("cleaning up mount", "path", loc)
		if err := syscall.Unmount(loc, 0); err != nil {
			return errors.Wrap(err, "error unmounting location: "+loc)
		}
	}
	return nil
}
