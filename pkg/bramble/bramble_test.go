package bramble

import (
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	TestTmpDirPrefix = "bramble-test-"
)

func tmpDir(t *testing.T) string {
	dir, err := ioutil.TempDir("/tmp", TestTmpDirPrefix)
	if err != nil {
		panic(err)
	}
	if t != nil {
		t.Cleanup(func() {
			os.RemoveAll(dir)
		})
	}
	return dir
}

func testfilesBramble(t *testing.T) *Bramble {
	b := Bramble{}
	if err := b.init("./testfiles", true); err != nil {
		t.Fatal(err)
	}
	return &b
}

func Test_parseModuleFuncArgument(t *testing.T) {
	b := testfilesBramble(t)
	tests := []struct {
		name       string
		args       []string
		wantModule string
		wantFn     string
		wantErr    string
	}{
		{
			name:       "reference by name and fn",
			args:       []string{"main:foo"},
			wantModule: "github.com/maxmcd/bramble/pkg/bramble/testfiles/main",
			wantFn:     "foo",
		}, {
			name:       "no path provided",
			args:       []string{":default"},
			wantModule: "github.com/maxmcd/bramble/pkg/bramble/testfiles",
			wantFn:     "default",
		}, {
			name:       "relative path to file",
			args:       []string{"bar/main:other"},
			wantModule: "github.com/maxmcd/bramble/pkg/bramble/testfiles/bar/main",
			wantFn:     "other",
		}, {
			name:       "full module name",
			args:       []string{"github.com/maxmcd/bramble/all:all"},
			wantModule: "github.com/maxmcd/bramble/all",
			wantFn:     "all",
		}, {
			name:       "relative path to file with slash",
			args:       []string{"./bar/main:other"},
			wantModule: "github.com/maxmcd/bramble/pkg/bramble/testfiles/bar/main",
			wantFn:     "other",
		}, {
			name:       "relative path to file with extension",
			args:       []string{"bar/main.bramble:other"},
			wantModule: "github.com/maxmcd/bramble/pkg/bramble/testfiles/bar/main",
			wantFn:     "other",
		}, {
			name:       "reference by subdirectory default",
			args:       []string{"foo:ok"},
			wantModule: "github.com/maxmcd/bramble/pkg/bramble/testfiles/foo",
			wantFn:     "ok",
		}, {
			name:       "reference by default fn",
			args:       []string{":default"},
			wantModule: "github.com/maxmcd/bramble/pkg/bramble/testfiles",
			wantFn:     "default",
		}, {
			name:    "missing file",
			args:    []string{"missing:foo"},
			wantErr: "no such file",
		}, {
			name:    "missing arg",
			args:    []string{},
			wantErr: "flag: help requested",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotModule, gotFn, err := b.parseModuleFuncArgument(tt.args)
			if (err != nil) && tt.wantErr != "" {
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("argsToImport() error doesn't match\nwanted:     %q\nto contain: %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Error(err)
			}
			if gotModule != tt.wantModule {
				t.Errorf("argsToImport() gotModule = %v, want %v", gotModule, tt.wantModule)
			}
			if gotFn != tt.wantFn {
				t.Errorf("argsToImport() gotFn = %v, want %v", gotFn, tt.wantFn)
			}
		})
	}
}

func TestBramble_resolveModule(t *testing.T) {
	b := testfilesBramble(t)
	defer func() { _ = os.Chdir("..") }()
	tests := []struct {
		name        string
		module      string
		wantGlobals []string
		wantErr     string
	}{
		{
			name:        "direct file import",
			module:      "github.com/maxmcd/bramble/pkg/bramble/testfiles/main",
			wantGlobals: []string{"foo"},
		}, {
			name:        "default directory import",
			module:      "github.com/maxmcd/bramble/pkg/bramble/testfiles",
			wantGlobals: []string{"default"},
		}, {
			name:        "ambiguous module without default.bramble in subfolder",
			module:      "github.com/maxmcd/bramble/pkg/bramble/testfiles/bar",
			wantGlobals: []string{"hello"},
		}, {
			name:    "missing file",
			module:  "github.com/maxmcd/bramble/pkg/bramble/testfiles/mayne",
			wantErr: "couldn't find",
		}, {
			name:    "missing default",
			module:  "github.com/maxmcd/bramble/pkg/bramble/",
			wantErr: "couldn't find",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotGlobals, err := b.resolveModule(tt.module)
			if (err != nil) && tt.wantErr != "" {
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Bramble.resolveModule() error doesn't match\nwanted:     %q\nto contain: %q", err, tt.wantErr)
				}
				return
			}
			globalNames := []string{}
			for key := range gotGlobals {
				globalNames = append(globalNames, key)
			}
			if !reflect.DeepEqual(globalNames, tt.wantGlobals) {
				t.Errorf("Bramble.resolveModule() = %v, want %v", globalNames, tt.wantGlobals)
			}
		})
	}
}

func TestBramble_moduleNameFromFileName(t *testing.T) {
	b := testfilesBramble(t)
	tests := []struct {
		filename       string
		module         string
		wantModuleName string
		wantErr        string
	}{
		{
			filename:       "bar.bramble",
			wantModuleName: "github.com/maxmcd/bramble/pkg/bramble/testfiles/bar",
		}, {
			filename: "noexist.bramble",
			wantErr:  "doesn't exist",
		}, {
			filename:       "default.bramble",
			wantModuleName: "github.com/maxmcd/bramble/pkg/bramble/testfiles",
		}, {
			filename:       "../../../tests/basic.bramble",
			wantModuleName: "github.com/maxmcd/bramble/tests/basic",
		},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			moduleName, err := b.moduleNameFromFileName(tt.filename)
			if (err != nil) && tt.wantErr != "" {
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Bramble.resolveModule() error doesn't match\nwanted:     %q\nto contain: %q", err, tt.wantErr)
				}
				return
			}
			assert.Equal(t, tt.wantModuleName, moduleName)
		})
	}
}
