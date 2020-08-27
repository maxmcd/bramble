package bramble

import (
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

var once sync.Once

func brambleBramble(t *testing.T) Bramble {
	once.Do(func() {
		if err := os.Chdir("./testfiles"); err != nil {
			t.Fatal(err)
		}
	})
	b := Bramble{}
	if err := b.init(); err != nil {
		t.Fatal(err)
	}
	return b
}

func Test_argsToImport(t *testing.T) {
	b := brambleBramble(t)
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
			name:       "reference by default fn",
			args:       []string{"default"},
			wantModule: "github.com/maxmcd/bramble/pkg/bramble/testfiles",
			wantFn:     "default",
		}, {
			name:    "missing file",
			args:    []string{"missing:foo"},
			wantErr: "doesn't exist",
		}, {
			name:    "mangled",
			args:    []string{"missing:foo:bar"},
			wantErr: "many colons",
		}, {
			name:    "missing arg",
			args:    []string{},
			wantErr: ErrRequiredFunctionArgument.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotModule, gotFn, err := b.argsToImport(tt.args)
			if (err != nil) && tt.wantErr != "" {
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("argsToImport() error doesn't match\nwanted:     %q\nto contain: %q", err, tt.wantErr)
				}
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

func TestBramble_resolveModule(t *testing.T) {
	b := brambleBramble(t)
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
			name:    "missing file",
			module:  "github.com/maxmcd/bramble/pkg/bramble/testfiles/mayne",
			wantErr: ErrModuleDoesNotExist.Error(),
		}, {
			name:    "missing default",
			module:  "github.com/maxmcd/bramble/pkg/bramble/",
			wantErr: ErrModuleDoesNotExist.Error(),
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
				t.Errorf("Bramble.resolveModule() = %v, want %v", gotGlobals, tt.wantGlobals)
			}
		})
	}
}
