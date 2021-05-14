package bramble_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/maxmcd/bramble/pkg/bramble"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/starutil"
)

type TestProject struct {
	bramblePath string
	projectPath string
}

var cachedProj *TestProject

func TestMain(m *testing.M) {
	var err error
	cachedProj, err = NewTestProject()
	if err != nil {
		fmt.Printf("%+v", err)
		panic(starutil.AnnotateError(err))
	}
	exitVal := m.Run()
	cachedProj.Cleanup()
	os.Exit(exitVal)
}

func (tp *TestProject) Copy() TestProject {
	out := TestProject{
		bramblePath: fileutil.TestTmpDir(nil),
		projectPath: fileutil.TestTmpDir(nil),
	}
	if err := fileutil.CopyDirectory(tp.bramblePath, out.bramblePath); err != nil {
		panic(err)
	}
	if err := fileutil.CopyDirectory(tp.projectPath, out.projectPath); err != nil {
		panic(err)
	}
	return out
}
func (tp *TestProject) Bramble() *bramble.Bramble {
	b, err := bramble.NewBramble(tp.projectPath, bramble.OptionNoRoot)
	if err != nil {
		panic(err)
	}
	// b.noRoot = true
	return b
}

func (tp *TestProject) Cleanup() {
	_ = os.RemoveAll(tp.bramblePath)
	_ = os.RemoveAll(tp.projectPath)
}

func NewTestProject() (*TestProject, error) {
	// Write files
	bramblePath := fileutil.TestTmpDir(nil)
	projectPath := fileutil.TestTmpDir(nil)

	if err := fileutil.CopyDirectory("./testdata", projectPath); err != nil {
		return nil, err
	}
	os.Setenv("BRAMBLE_PATH", bramblePath)

	// Init bramble
	b, err := bramble.NewBramble(projectPath, bramble.OptionNoRoot)
	if err != nil {
		return nil, err
	}
	// b.noRoot = true
	ctx := context.Background()
	if err := b.Build(ctx, []string{":busybox"}); err != nil {
		return nil, err
	}
	return &TestProject{
		bramblePath: bramblePath,
		projectPath: projectPath,
	}, nil
}

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
