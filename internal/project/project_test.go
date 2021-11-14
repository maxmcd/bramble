package project

import (
	"strings"
	"testing"

	"github.com/maxmcd/bramble/pkg/fxt"
	"github.com/maxmcd/bramble/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProject(t *testing.T) {
	{
		p, err := NewProject(".")
		require.NoError(t, err)
		assert.Equal(t, p.config.Package.Name, "github.com/maxmcd/bramble")
	}
	{
		p, err := NewProject("./testdata/project")
		require.NoError(t, err)
		assert.Equal(t, p.config.Package.Name, "testproject")
		writer := p.LockfileWriter()
		if err := writer.AddEntry("foo", "bar"); err != nil {
			t.Fatal(err)
		}
		if err := p.WriteLockfile(); err != nil {
			t.Fatal(err)
		}
	}
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
			[]string{"github.com/maxmcd/bramble/lib"},
			nil, false,
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
			arg:        "main:foo",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/main",
			wantFn:     "foo",
		}, {
			name:       "no path provided",
			arg:        ":default",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata",
			wantFn:     "default",
		}, {
			name:       "relative path to file",
			arg:        "bar/main:other",
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
			arg:        "bar/main.bramble:other",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/bar/main",
			wantFn:     "other",
		}, {
			name:       "reference by subdirectory default",
			arg:        "foo:ok",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/foo",
			wantFn:     "ok",
		}, {
			name:       "reference by subdirectory default with no function",
			arg:        "foo",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/foo",
		}, {
			name:       "reference by default fn",
			arg:        ":default",
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata",
			wantFn:     "default",
		}, {
			name:    "missing file",
			arg:     "missing:foo",
			wantErr: "no such file",
		}, {
			name:    "missing arg",
			arg:     "",
			wantErr: "module name can't be blank",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotModule, gotFn, err := p.parseModuleFuncArgument(tt.arg)
			if test.ErrContains(t, err, tt.wantErr) {
				return
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

func TestProject_BuildArgumentsToModules(t *testing.T) {
	tests := []struct {
		name        string
		wd          string
		args        []string
		wantModules []string
		wantErr     bool
	}{
		{"simple", "./testdata/project", []string{"./..."}, []string{"testproject/a", "testproject"}, false},
		{"simple", ".", []string{"./testdata"}, []string{"./testdata"}, false},
		// TODO: this just expands ./... and then accepts all other arguments
		// as-is. Will want to expand this if we move more validation logic into
		// this fn
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestProject(t, tt.wd)
			gotModules, err := p.BuildArgumentsToModules(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Project.BuildArgumentsToModules() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, gotModules, tt.wantModules)
		})
	}
}
