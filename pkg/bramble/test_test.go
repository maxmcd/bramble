package bramble

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/starutil"
)

var (
	scriptSh = `
set -e
$busybox_download/busybox-x86_64 mkdir $out/bin
$busybox_download/busybox-x86_64 cp $busybox_download/busybox-x86_64 $out/bin/busybox
cd $out/bin
for command in $(./busybox --list); do
	./busybox ln -s busybox $command
done
`
	busybox = `
def fetch_url(url):
    return derivation(name="fetch-url", builder="fetch_url", env={"url": url})

def busybox():
    b = fetch_url("https://brmbl.s3.amazonaws.com/busybox-x86_64.tar.gz")

    return derivation(
        name="busybox",
        builder=b.out + "/busybox-x86_64",
        args=["sh", "./script.sh"],
		sources=files(["./script.sh"]),
        env={"busybox_download": b},
    )`
	brambleToml = `
[module]
name = "github.com/maxmcd/bramble"`
)

type TestProject struct {
	bramblePath string
	projectPath string
	oldWD       string
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
		bramblePath: tmpDir(nil),
		projectPath: tmpDir(nil),
	}
	fmt.Println(tp.bramblePath, out.bramblePath)
	fmt.Println(tp.projectPath, out.projectPath)
	if err := fileutil.CopyDirectory(tp.bramblePath, out.bramblePath); err != nil {
		panic(err)
	}
	if err := fileutil.CopyDirectory(tp.projectPath, out.projectPath); err != nil {
		panic(err)
	}
	return out
}
func (tp *TestProject) Bramble() *Bramble {
	b := &Bramble{}
	b.noRoot = true
	return b
}
func (tp *TestProject) Chdir() {
	tp.oldWD, _ = os.Getwd()
	_ = os.Chdir(tp.projectPath)
}
func (tp *TestProject) Cleanup() {
	_ = os.RemoveAll(tp.bramblePath)
	_ = os.RemoveAll(tp.projectPath)
	if tp.oldWD != "" {
		_ = os.Chdir(tp.oldWD)
	}
}

func NewTestProject() (*TestProject, error) {
	b := Bramble{}
	b.noRoot = true
	bramblePath := tmpDir(nil)
	projectPath := tmpDir(nil)
	ctx := context.Background()
	if err := ioutil.WriteFile(
		filepath.Join(projectPath, "./default.bramble"),
		[]byte(busybox), 0644); err != nil {
		return nil, err
	}
	if err := ioutil.WriteFile(
		filepath.Join(projectPath, "./script.sh"),
		[]byte(scriptSh), 0644); err != nil {
		return nil, err
	}
	if err := ioutil.WriteFile(
		filepath.Join(projectPath, "./bramble.toml"),
		[]byte(brambleToml), 0644); err != nil {
		return nil, err
	}
	os.Setenv("BRAMBLE_PATH", bramblePath)
	wd, _ := os.Getwd()
	_ = os.Chdir(projectPath)
	if err := b.build(ctx, []string{":busybox"}); err != nil {
		return nil, err
	}
	_ = os.Chdir(wd)
	return &TestProject{
		bramblePath: bramblePath,
		projectPath: projectPath,
	}, nil
}
