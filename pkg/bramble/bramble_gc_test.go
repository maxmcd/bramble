package bramble

import (
	"context"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxmcd/bramble/pkg/starutil"
)

func TestFoo(t *testing.T) {
	tp := cachedProj.Copy()
	t.Cleanup(tp.Cleanup)

	if err := ioutil.WriteFile(
		filepath.Join(tp.projectPath, "./foo.bramble"),
		[]byte(`
load("github.com/maxmcd/bramble")

def ok():
	return bramble.busybox()
		`), 0644); err != nil {
		t.Fatal(err)
	}
	tp.Chdir()
	if err := tp.Bramble().build(context.Background(), []string{"foo:ok"}); err != nil {
		t.Fatal(err)
	}
}

func TestDependency(t *testing.T) {
	tp := cachedProj.Copy()
	// t.Cleanup(tp.Cleanup)
	if err := ioutil.WriteFile(
		filepath.Join(tp.projectPath, "./dep.bramble"),
		[]byte(`
load("github.com/maxmcd/bramble")

def ok():
	bb = bramble.busybox()
	return derivation(
		builder=bb.out + "/bin/sh",
		env=dict(PATH=bb.out+"/bin", bb=bb.out),
		args=["-c", """
		set -e

		mkdir -p $out/bin
		touch $out/bin/say-hi
		chmod +x $out/bin/say-hi

		echo "#!$bb/bin/sh" > $out/bin/say-hi
		echo "$bb/bin/echo hi" >> $out/bin/say-hi

		$out/bin/say-hi
		"""]
	)
		`), 0644); err != nil {
		t.Fatal(err)
	}
	tp.Chdir()
	if err := tp.Bramble().build(context.Background(), []string{"dep:ok"}); err != nil {
		t.Fatal(err)
	}
	fsys := os.DirFS(tp.bramblePath)
	storeEntries, err := fs.ReadDir(fsys, "store")
	if err != nil {
		t.Error(err)
	}
	for _, entry := range storeEntries {
		if strings.Contains(entry.Name(), "bramble_build_directory") {
			t.Error("found build directory in store", entry.Name())
		}
	}
	if err := tp.Bramble().gc(nil); err != nil {
		fmt.Println(starutil.AnnotateError(err))
		t.Fatal(err)
	}
}
