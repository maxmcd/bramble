package bramble_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func NewTestProject(t *testing.T) *TestProject {
	tp := cachedProj.Copy()
	// t.Cleanup(tp.Cleanup)
	return tp
}

func TestFoo(t *testing.T) {
	tp := NewTestProject(t)

	if err := ioutil.WriteFile(
		filepath.Join(tp.projectPath, "./foo.bramble"),
		[]byte(`
load("github.com/maxmcd/bramble")

def ok():
	return bramble.busybox()
		`), 0644); err != nil {
		t.Fatal(err)
	}
	_, result, err := tp.Bramble().Build(context.Background(), []string{"foo:ok"})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(result)
}

func TestOneByOne(t *testing.T) {
	tp := NewTestProject(t)
	drvs, result, err := tp.Bramble().Build(context.Background(), []string{":fetch_busybox"})
	require.NoError(t, err)
	fmt.Println(drvs, result)
	{
		drvs, result, err := tp.Bramble().Build(context.Background(), []string{":busybox"})
		require.NoError(t, err)
		fmt.Println(drvs[0].PrettyJSON(), result)
	}
}

func TestDependency(t *testing.T) {
	tp := NewTestProject(t)
	if err := tp.Bramble().GC(nil); err != nil {
		fmt.Printf("%+v", err)
		fmt.Println(starutil.AnnotateError(err))
		t.Fatal(err)
	}
	b := tp.Bramble()
	drvs, result, err := b.Build(context.Background(), []string{"dep:hello_world"})
	if err != nil {
		t.Fatal(err, starutil.AnnotateError(err))
	}

	fmt.Println(result)

	for _, build := range result {
		switch build.Derivation.Name {
		case "fetch-url":
			assert.Equal(t, build.DidBuild, false, build.Derivation.Name)
		default:
			assert.Equal(t, build.DidBuild, true, build.Derivation.Name)
		}
	}
	drv := drvs[0]
	{
		graph, err := drv.BuildDependencyGraph()
		require.NoError(t, err)
		graph.PrintDot()
	}

	{
		graph, err := drv.RuntimeDependencyGraph()
		require.NoError(t, err)
		graph.PrintDot()
	}
	{
		if err := tp.Bramble().GC(nil); err != nil {
			fmt.Printf("%+v", err)
			fmt.Println(starutil.AnnotateError(err))
			t.Fatal(err)
		}
		b := tp.Bramble()
		_, result, err := b.Build(context.Background(), []string{"dep:hello_world"})
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println(result)
		for _, build := range result {
			// shouldn't need to rebuild anything after a GC calls
			assert.False(t, build.DidBuild)
		}
	}
}
