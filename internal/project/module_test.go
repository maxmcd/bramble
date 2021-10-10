package project

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_parseModuleFuncArgument(t *testing.T) {
	rt := newTestRuntime(t, "")
	rt.workingDirectory = filepath.Join(rt.workingDirectory, "testdata")

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
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/main",
			wantFn:     "foo",
		}, {
			name:       "no path provided",
			args:       []string{":default"},
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata",
			wantFn:     "default",
		}, {
			name:       "relative path to file",
			args:       []string{"bar/main:other"},
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/bar/main",
			wantFn:     "other",
		}, {
			name:       "full module name",
			args:       []string{"github.com/maxmcd/bramble/all:all"},
			wantModule: "github.com/maxmcd/bramble/all",
			wantFn:     "all",
		}, {
			name:       "relative path to file with slash",
			args:       []string{"./bar/main:other"},
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/bar/main",
			wantFn:     "other",
		}, {
			name:       "relative path to file with extension",
			args:       []string{"bar/main.bramble:other"},
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/bar/main",
			wantFn:     "other",
		}, {
			name:       "reference by subdirectory default",
			args:       []string{"foo:ok"},
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/foo",
			wantFn:     "ok",
		}, {
			name:       "reference by subdirectory default with no function",
			args:       []string{"foo"},
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata/foo",
		}, {
			name:       "reference by default fn",
			args:       []string{":default"},
			wantModule: "github.com/maxmcd/bramble/internal/project/testdata",
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
			gotModule, gotFn, err := rt.parseModuleFuncArgument(tt.args)
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
			if !reflect.DeepEqual(globalNames, tt.wantGlobals) {
				t.Errorf("Bramble.resolveModule() = %v, want %v", globalNames, tt.wantGlobals)
			}
		})
	}
}

func TestBramble_moduleNameFromFileName(t *testing.T) {
	rt := newTestRuntime(t, "")
	rt.workingDirectory = filepath.Join(rt.workingDirectory, "testdata")

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
			moduleName, err := rt.moduleNameFromFileName(tt.filename)
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

// TODO: move to its own module so that the regular module can build entirely
// func TestCircularImport(t *testing.T) {
// 	rt := newTestRuntime(t)
// 	_, err := rt.execModule("github.com/maxmcd/bramble/internal/project/testdata/circular/a")
// 	require.Error(t, err)
// 	assert.Contains(t, err.Error(), "cycle in load graph")
// }
