package bramble_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
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
	if err := tp.Bramble().Build(context.Background(), []string{"foo:ok"}); err != nil {
		t.Fatal(err)
	}
}

func TestDependency(t *testing.T) {
	tp := cachedProj.Copy()
	t.Cleanup(tp.Cleanup)
	if err := tp.Bramble().GC(nil); err != nil {
		fmt.Printf("%+v", err)
		fmt.Println(starutil.AnnotateError(err))
		t.Fatal(err)
	}
	// {
	// 	b := tp.Bramble()
	// 	if err := b.Build(context.Background(), []string{"dep:ok"}); err != nil {
	// 		t.Fatal(err)
	// 	}
	// 	var drv *bramble.Derivation
	// 	b.derivations.Range(func(filename string, d *bramble.Derivation) bool {
	// 		if d.Name == "ok" {
	// 			drv = d
	// 			return false
	// 		}
	// 		return true
	// 	})
	// 	{
	// 		graph, err := drv.buildDependencies()
	// 		require.NoError(t, err)
	// 		graph.PrintDot()
	// 		fmt.Println(graph.String(), "----")
	// 	}
	// 	{
	// 		graph, err := drv.runtimeDependencyGraph()
	// 		require.NoError(t, err)
	// 		graph.PrintDot()
	// 		fmt.Println(graph.String(), "----")
	// 	}
	// 	fmt.Println(drv.inputFiles())
	// 	fmt.Println(drv.runtimeDependencies())
	// 	fmt.Println(drv.runtimeFiles("out"))
	// 	fsys := os.DirFS(tp.bramblePath)
	// 	storeEntries, err := fs.ReadDir(fsys, "store")
	// 	if err != nil {
	// 		t.Error(err)
	// 	}
	// 	for _, entry := range storeEntries {
	// 		if strings.Contains(entry.Name(), "bramble_build_directory") {
	// 			t.Error("found build directory in store", entry.Name())
	// 		}
	// 	}
	// }
	// if err := tp.Bramble().gc(nil); err != nil {
	// 	fmt.Printf("%+v", err)
	// 	fmt.Println(starutil.AnnotateError(err))
	// 	t.Fatal(err)
	// }
	// fmt.Println(tp)
}
