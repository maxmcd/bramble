package chunkedarchive

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/stretchr/testify/require"
)

func TestArchive(t *testing.T) {
	tests := []struct {
		name string
		in   []entry
	}{
		{
			name: "one file",
			in:   entries(file("foo")),
		},
		{
			name: "files and dir",
			in: entries(
				dir("thing"),
				file("foo"),
				emptyFile("empty"),
			),
		},
		{
			name: "nested files",
			in: entries(
				dir("thing"),
				file("foo"),
				file("thing/bar"),
			),
		},
		{
			name: "unusual suspects",
			in: entries(
				dir("thing"),
				file("thing/bar"),
				symlink("thing/bar", "symlink"),
				hardlink("thing/bar", "hardlink"),
				fifo("thing/pipe"),
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, e := range tt.in {
				if err := e(dir); err != nil {
					t.Fatal(err)
				}
			}
			f, err := os.CreateTemp("", "")
			require.NoError(t, err)
			f.Close()
			t.Cleanup(func() { os.Remove(f.Name()) })

			if err := FileArchive(dir, f.Name()); err != nil {
				t.Fatal(err)
			}
			dir2 := t.TempDir()

			if err := FileUnarchive(f.Name(), dir2); err != nil {
				t.Fatal(err)
			}

			if reptarDir(t, dir) != reptarDir(t, dir2) {
				{
					cmd := "diff -qr " + dir + " " + dir2
					b, err := exec.Command("bash", "-c", cmd).CombinedOutput()
					fmt.Println(string(b))
					if err != nil {
						fmt.Println(cmd)
					}
					require.NoError(t, err)
				}
				cmd := "git diff --color=never --no-index " + dir + " " + dir2
				b, err := exec.Command("bash", "-c", cmd).CombinedOutput()
				if err != nil {
					fmt.Println(cmd)
					fmt.Println(string(b))
				}
				require.NoError(t, err)
			}
		})
	}
}

func reptarDir(t *testing.T, location string) string {
	h := hasher.New()
	if err := reptar.Reptar(location, h); err != nil {
		t.Fatal(err)
	}
	return h.String()
}

type entry func(dir string) error

func entries(v ...entry) []entry {
	return v
}

var j func(...string) string = filepath.Join

func file(name string) func(dir string) error {
	return func(dir string) error {
		f, err := os.Create(j(dir, name))
		if err != nil {
			return err
		}
		if _, err := io.CopyN(f, rand.Reader, 9e6); err != nil {
			return err
		}
		return f.Close()
	}
}

func emptyFile(name string) func(dir string) error {
	return func(dir string) error {
		f, err := os.Create(j(dir, name))
		if err != nil {
			return err
		}
		return f.Close()
	}
}

func dir(name string) func(dir string) error {
	return func(dir string) error {
		return os.Mkdir(j(dir, name), 0o755)
	}
}

func hardlink(name, link string) func(dir string) error {
	return func(dir string) error {
		return os.Link(j(dir, name), j(dir, link))
	}
}

func symlink(name, link string) func(dir string) error {
	return func(dir string) error {
		return os.Symlink(j(dir, name), j(dir, link))
	}
}

func fifo(name string) func(dir string) error {
	return func(dir string) error {
		return syscall.Mkfifo(j(dir, name), 0o755)
	}
}
