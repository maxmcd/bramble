package project

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProject(t *testing.T) {
	{
		p, err := NewProject(".")
		require.NoError(t, err)
		assert.Equal(t, p.config.Module.Name, "github.com/maxmcd/bramble")
	}
	{
		p, err := NewProject("./testdata/project")
		require.NoError(t, err)
		assert.Equal(t, p.config.Module.Name, "testproject")
		writer := p.LockfileWriter()
		if err := writer.AddEntry("foo", "bar"); err != nil {
			t.Fatal(err)
		}
		if err := p.WriteLockfile(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProject_ListFunctions(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantM   ModuleDoc
		wantErr bool
	}{
		{
			name: "foo",
			path: "./testdata/main.bramble",
			wantM: ModuleDoc{
				Name:      "github.com/maxmcd/bramble/internal/project/testdata/main",
				Docstring: "\"\"\"Hello this is the main module\"\"\"",
				Functions: []FunctionDoc{
					{
						Docstring:  "\"\"\"foo is a tricky fellow. sure to return good news, but also the bitter taste of regret\"\"\"",
						Name:       "foo",
						Definition: "def foo()",
					}, {
						Docstring:  "\"this is just a string\"",
						Name:       "thing",
						Definition: "def thing(hi, foo=None, bar=[1, 2, {\"foo\": 5}, dict(ho=\"hum\")], out={})",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewProject(".")
			require.NoError(t, err)
			gotM, err := p.parsedModuleDocFromPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Project.ListFunctions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotM, tt.wantM) {
				require.Equal(t, gotM, tt.wantM)
			}
		})
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
		{"../../", "./tests",
			[]string{"github.com/maxmcd/bramble/tests/basic"},
			[]string{"github.com/maxmcd/bramble"}, false},
		{"../../", ".",
			[]string{"github.com/maxmcd/bramble/lib"}, nil, false},
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
