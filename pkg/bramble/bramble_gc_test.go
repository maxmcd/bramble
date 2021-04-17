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
	"github.com/stretchr/testify/require"
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

// 2puwgpqcw7fjf4vqsah6ukhvrfk235yk

func TestDependency(t *testing.T) {
	tp := cachedProj.Copy()
	// t.Cleanup(tp.Cleanup)
	if err := ioutil.WriteFile(
		filepath.Join(tp.projectPath, "./dep.bramble"),
		[]byte(`
load("github.com/maxmcd/bramble")

def busy_wrap():
	bb = bramble.busybox()
	return derivation(
		name="busy_wrap",
		outputs=["out","docs"],
		builder=bb.out + "/bin/sh",
		env=dict(PATH=bb.out+"/bin", bb=bb.out),
		args=["-c", """
		set -e

		cp -r $bb/bin $out/

		echo "here are the docs" > $docs/doc.txt
		"""]
	)

def ok():
	bb = busy_wrap()
	return derivation(
		name="ok",
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
	if err := tp.Bramble().gc(nil); err != nil {
		fmt.Printf("%+v", err)
		fmt.Println(starutil.AnnotateError(err))
		t.Fatal(err)
	}
	{
		b := tp.Bramble()
		if err := b.build(context.Background(), []string{"dep:ok"}); err != nil {
			t.Fatal(err)
		}
		var drv *Derivation
		b.derivations.Range(func(filename string, d *Derivation) bool {
			if d.Name == "ok" {
				drv = d
				return false
			}
			return true
		})
		{
			graph, err := drv.buildDependencies()
			require.NoError(t, err)
			graph.PrintDot()
			fmt.Println(graph.String(), "----")
		}
		{
			graph, err := drv.runtimeDependencyGraph()
			require.NoError(t, err)
			graph.PrintDot()
			fmt.Println(graph.String(), "----")
		}
		fmt.Println(drv.inputFiles())
		fmt.Println(drv.runtimeDependencies())
		fmt.Println(drv.runtimeFiles("out"))
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
	}
	if err := tp.Bramble().gc(nil); err != nil {
		fmt.Printf("%+v", err)
		fmt.Println(starutil.AnnotateError(err))
		t.Fatal(err)
	}
	fmt.Println(tp)
}
