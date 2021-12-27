package project

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/maxmcd/bramble/pkg/fxt"
	"github.com/maxmcd/bramble/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBramble_resolveModule(t *testing.T) {
	rt := newTestRuntime(t, "")
	tests := []struct {
		name        string
		module      string
		wantGlobals []string
		wantErr     string
	}{
		{
			name:        "direct file import",
			module:      "github.com/maxmcd/bramble/internal/project/testdata/main",
			wantGlobals: []string{"foo", "thing"},
		}, {
			name:        "default directory import",
			module:      "github.com/maxmcd/bramble/internal/project/testdata",
			wantGlobals: []string{"default"},
		}, {
			name:        "ambiguous module without default.bramble in subfolder",
			module:      "github.com/maxmcd/bramble/internal/project/testdata/bar",
			wantGlobals: []string{"hello"},
		}, {
			name:    "missing file",
			module:  "github.com/maxmcd/bramble/internal/project/testdata/mayne",
			wantErr: "does not exist",
		}, {
			name:    "missing default",
			module:  "github.com/maxmcd/bramble/pkg/bramble/",
			wantErr: "does not exist",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotGlobals, err := rt.execModule(context.Background(), tt.module)
			if (err != nil) && tt.wantErr != "" {
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Bramble.execModule() error doesn't match\nwanted:     %q\nto contain: %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Error(err)
			}
			globalNames := []string{}
			for key := range gotGlobals {
				globalNames = append(globalNames, key)
			}
			sort.Strings(globalNames)
			sort.Strings(tt.wantGlobals)
			if !reflect.DeepEqual(globalNames, tt.wantGlobals) {
				t.Errorf("Bramble.resolveModule() = %v, want %v", globalNames, tt.wantGlobals)
			}
		})
	}
}

func TestCircularImport(t *testing.T) {
	p := newTestProject(t, "./testdata/circular")
	fmt.Println(p.location)
	_, err := p.ExecModule(context.Background(),
		ExecModuleInput{
			Module: Module{
				Name: "github.com/maxmcd/bramble/internal/project/testdata/circular/a",
			},
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle in load graph")
}

func TestProject_FindAllModules(t *testing.T) {
	tests := []struct {
		wd                    string
		path                  string
		modulesContains       []string
		modulesDoesNotContain []string
		wantErr               bool
	}{
		{
			"../../", "./tests",
			[]string{"github.com/maxmcd/bramble/tests/basic"},
			[]string{"github.com/maxmcd/bramble"},
			false,
		},
		{
			"../../", ".",
			[]string{"github.com/maxmcd/bramble/tests"},
			[]string{
				"github.com/maxmcd/bramble/internal/project/testdata/circular/b",
				"github.com/maxmcd/bramble/internal/project/testdata/circular",
				"github.com/maxmcd/bramble/internal/project/testdata/circular/a"},
			false,
		},
		{"../../../", ".", nil, nil, true},
		{"./testdata/project", ".", []string{"testproject/a"}, nil, false},
		{"./testdata/", ".", nil, []string{
			"github.com/maxmcd/bramble/internal/project/testdata/project/a",
			"github.com/maxmcd/bramble/internal/project/testdata/project/",
			"github.com/maxmcd/bramble/internal/project/testdata/project",
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.wd+"&"+tt.path, func(t *testing.T) {
			p, err := NewProject(tt.wd)
			if err != nil && tt.wantErr {
				return
			}
			gotModules, err := p.FindAllModules(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Project.FindAllModules() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			for _, m := range tt.modulesContains {
				require.Contains(t, gotModules, m)
			}
			for _, m := range tt.modulesDoesNotContain {
				require.NotContains(t, gotModules, m)
			}
		})
	}
}

func TestProject_scanForLoadNames(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewProject("")
			if err != nil {
				t.Fatal(err)
			}
			names, err := p.scanForLoadNames()
			if (err != nil) != tt.wantErr {
				t.Errorf("Project.scanForLoadNames() error = %v, wantErr %v", err, tt.wantErr)
			}
			fxt.Printqln(names)
			// TODO: Assert something
		})
	}
}

func TestBramble_moduleNameFromFileName(t *testing.T) {
	p := newTestProject(t, "./testdata")
	tests := []struct {
		filename       string
		module         string
		wantModuleName string
		wantErr        string
	}{
		{
			filename:       "bar.bramble",
			wantModuleName: "github.com/maxmcd/bramble/internal/project/testdata/bar",
		}, {
			filename: "noexist.bramble",
			wantErr:  "doesn't exist",
		}, {
			filename:       "default.bramble",
			wantModuleName: "github.com/maxmcd/bramble/internal/project/testdata",
		}, {
			filename:       "../../../tests/basic.bramble",
			wantModuleName: "github.com/maxmcd/bramble/tests/basic",
		},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			moduleName, err := p.moduleNameFromFileName(tt.filename)
			if (err != nil) && tt.wantErr != "" {
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Bramble.resolveModule() error doesn't match\nwanted:     %q\nto contain: %q", err, tt.wantErr)
				}
				return
			} else if err != nil {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantModuleName, moduleName)
		})
	}
}

func Test_parseModuleFuncArgument(t *testing.T) {
	p := newTestProject(t, "./testdata")

	tests := []struct {
		name       string
		arg        string
		wantModule string
		wantFn     string
		wantErr    string
	}{
		{
			name:       "reference by name and fn",
			arg:        "./main:foo",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/main",
			wantFn:     "foo",
		}, {
			name:       "no path provided",
			arg:        "./:default",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata",
			wantFn:     "default",
		}, {
			name:       "relative path to file",
			arg:        "./bar/main:other",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/bar/main",
			wantFn:     "other",
		}, {
			name:       "full module name",
			arg:        "github.com/maxmcd/bramble:all",
			wantModule: "github.com/maxmcd/bramble",
			wantFn:     "all",
		}, {
			name:       "relative path to file with slash",
			arg:        "./bar/main:other",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/bar/main",
			wantFn:     "other",
		}, {
			name:       "relative path to file with extension",
			arg:        "./bar/main.bramble:other",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/bar/main",
			wantFn:     "other",
		}, {
			name:       "reference by subdirectory default",
			arg:        "./foo:ok",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/foo",
			wantFn:     "ok",
		}, {
			name:       "reference by subdirectory default with no function",
			arg:        "./foo",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/foo",
		}, {
			name:       "reference by default fn",
			arg:        "./:default",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata",
			wantFn:     "default",
		}, {
			name:    "missing file",
			arg:     "missing:foo",
			wantErr: "not a dependency",
		}, {
			name:    "missing file",
			arg:     "./missing:foo",
			wantErr: "no such file",
		}, {
			name:    "missing arg",
			arg:     "",
			wantErr: "module name can't be blank",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module, err := p.ParseModuleFuncArgument(context.Background(), tt.arg, false)
			if test.ErrContains(t, err, tt.wantErr) {
				return
			}
			if module.Name != tt.wantModule {
				t.Errorf("argsToImport() gotModule = %v, want %v", module.Name, tt.wantModule)
			}
			if module.Function != tt.wantFn {
				t.Errorf("argsToImport() module.Function = %v, want %v", module.Function, tt.wantFn)
			}
		})
	}
}

func TestProject_BuildArgumentsToModules(t *testing.T) {
	tests := []struct {
		name        string
		wd          string
		args        []string
		wantModules []Module
		wantErr     bool
	}{
		{
			name: "simple",
			wd:   "./testdata/project",
			args: []string{"./..."},
			wantModules: []Module{
				{"testproject/a", "", false},
				{"testproject", "", false},
			},
			wantErr: false,
		},
		{
			name: "simple",
			wd:   ".",
			args: []string{"./testdata"},
			wantModules: []Module{
				{"github.com/maxmcd/bramble/internal/project/testdata", "", false},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestProject(t, tt.wd)
			gotModules, err := p.ArgumentsToModules(context.Background(), tt.args, false)
			if (err != nil) != tt.wantErr {
				t.Errorf("Project.BuildArgumentsToModules() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, gotModules, tt.wantModules)
		})
	}
}
