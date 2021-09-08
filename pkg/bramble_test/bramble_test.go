package bramble_test

// import (
// 	"context"
// 	"fmt"
// 	"io/ioutil"
// 	"path/filepath"
// 	"testing"

// 	"github.com/maxmcd/bramble/pkg/bramble"
// 	"github.com/maxmcd/bramble/pkg/fileutil"
// 	"github.com/maxmcd/bramble/pkg/starutil"
// 	"github.com/stretchr/testify/require"
// )

// func NewTestProject(t *testing.T) *TestProject {
// 	tp := cachedProj.Copy()
// 	t.Cleanup(tp.Cleanup)
// 	return tp
// }

// func TestFoo(t *testing.T) {
// 	tp := NewTestProject(t)

// 	if err := ioutil.WriteFile(
// 		filepath.Join(tp.projectPath, "./foo.bramble"),
// 		[]byte(`
// load("github.com/maxmcd/bramble")

// def ok():
// 	return bramble.busybox()
// 		`), 0644); err != nil {
// 		t.Fatal(err)
// 	}
// 	_, result, err := tp.Bramble().Build(context.Background(), []string{"foo:ok"})
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	fmt.Println(result)
// }

// func ensureResult(t *testing.T, result []bramble.BuildResult, v map[string]bool) {
// 	if len(result) != len(v) {
// 		t.Error("result and map have different lengths", result, v)
// 		return
// 	}
// 	for _, r := range result {
// 		br, ok := v[r.Derivation.Name]
// 		if !ok {
// 			t.Errorf("%q present in result but not in values", r.Derivation.Name)
// 			return
// 		}
// 		if br != r.DidBuild {
// 			t.Errorf("derivcation %q DidBuild was supposed to be %t but it was %t", r.Derivation.Name, br, r.DidBuild)
// 		}
// 	}
// }

// func TestEarlyCutoff(t *testing.T) {
// 	tp := NewTestProject(t)
// 	{
// 		_, result, err := tp.Bramble().Build(context.Background(), []string{"dep:hello_world"})
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		ensureResult(t, result, map[string]bool{
// 			"fetch-url":   false,
// 			"busybox":     true,
// 			"say_world":   true,
// 			"say_hello":   true,
// 			"hello_world": true,
// 		})
// 	}

// 	// Change the file contents with a comment
// 	err := fileutil.ReplaceAll(tp.projectPath+"/dep.bramble",
// 		"touch $out/bin/say-world",
// 		"touch $out/bin/say-world\n# random random random")
// 	require.NoError(t, err)

// 	{
// 		_, result, err := tp.Bramble().Build(context.Background(), []string{"dep:hello_world"})
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		ensureResult(t, result, map[string]bool{
// 			"fetch-url":   false,
// 			"busybox":     false,
// 			"say_world":   true, // only the changed drv should rebuild
// 			"say_hello":   false,
// 			"hello_world": false,
// 		})
// 	}
// }

// func TestOneByOne(t *testing.T) {
// 	tp := NewTestProject(t)
// 	drvs, result, err := tp.Bramble().Build(context.Background(), []string{":fetch_busybox"})
// 	require.NoError(t, err)
// 	fmt.Println(drvs, result)
// 	{
// 		drvs, result, err := tp.Bramble().Build(context.Background(), []string{":busybox"})
// 		require.NoError(t, err)
// 		fmt.Println(drvs[0].PrettyJSON(), result)
// 	}
// }

// func TestDependency(t *testing.T) {
// 	tp := NewTestProject(t)
// 	if err := tp.Bramble().GC(nil); err != nil {
// 		fmt.Printf("%+v", err)
// 		fmt.Println(starutil.AnnotateError(err))
// 		t.Fatal(err)
// 	}
// 	b := tp.Bramble()
// 	drvs, result, err := b.Build(context.Background(), []string{"dep:hello_world"})
// 	if err != nil {
// 		t.Fatal(err, starutil.AnnotateError(err))
// 	}

// 	ensureResult(t, result, map[string]bool{
// 		"fetch-url":   false,
// 		"busybox":     true,
// 		"say_world":   true,
// 		"say_hello":   true,
// 		"hello_world": true,
// 	})

// 	drv := drvs[0]
// 	{
// 		graph, err := drv.BuildDependencyGraph()
// 		require.NoError(t, err)
// 		graph.PrintDot()
// 	}

// 	{
// 		graph, err := drv.RuntimeDependencyGraph()
// 		require.NoError(t, err)
// 		graph.PrintDot()
// 	}
// 	{
// 		if err := tp.Bramble().GC(nil); err != nil {
// 			fmt.Printf("%+v", err)
// 			fmt.Println(starutil.AnnotateError(err))
// 			t.Fatal(err)
// 		}
// 		b := tp.Bramble()
// 		_, result, err := b.Build(context.Background(), []string{"dep:hello_world"})
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		ensureResult(t, result, map[string]bool{
// 			"fetch-url":   false,
// 			"busybox":     false,
// 			"say_world":   false,
// 			"say_hello":   false,
// 			"hello_world": false,
// 		})
// 	}
// }
