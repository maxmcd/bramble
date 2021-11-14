package project

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
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

// TODO: move to its own module so that the regular module can build entirely
// func TestCircularImport(t *testing.T) {
// 	rt := newTestRuntime(t)
// 	_, err := rt.execModule("github.com/maxmcd/bramble/internal/project/testdata/circular/a")
// 	require.Error(t, err)
// 	assert.Contains(t, err.Error(), "cycle in load graph")
// }
