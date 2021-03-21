package bramble

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"
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
