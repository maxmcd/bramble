package bramble_test

import (
	"context"
	"fmt"
	"os"
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
	cachedProj, err = newTestProject()
	if err != nil {
		fmt.Printf("%+v", err)
		panic(starutil.AnnotateError(err))
	}
	exitVal := m.Run()
	cachedProj.Cleanup()
	os.Exit(exitVal)
}

func (tp *TestProject) Copy() *TestProject {
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
	return &out
}
func (tp *TestProject) Bramble() *bramble.Bramble {
	b, err := bramble.NewBramble(tp.projectPath,
		bramble.OptionNoRoot,
		bramble.OptionBramblePath(tp.bramblePath))
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

func newTestProject() (*TestProject, error) {
	// Write files
	bramblePath := fileutil.TestTmpDir(nil)
	projectPath := fileutil.TestTmpDir(nil)

	if err := fileutil.CopyDirectory("./testdata", projectPath); err != nil {
		return nil, err
	}

	// Init bramble
	b, err := bramble.NewBramble(projectPath,
		bramble.OptionNoRoot,
		bramble.OptionBramblePath(bramblePath))
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	if _, _, err = b.Build(ctx, []string{":fetch_busybox"}); err != nil {
		return nil, err
	}
	return &TestProject{
		bramblePath: bramblePath,
		projectPath: projectPath,
	}, nil
}
