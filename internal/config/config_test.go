package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig_LoadValueToDependency(t *testing.T) {
	tests := []struct {
		name string
		deps map[string]Dependency
		val  string
		want string
	}{
		{
			name: "basic",
			deps: map[string]Dependency{
				"github.com/maxmcd/bramble":             {},
				"github.com/maxmcd/bramble/tests":       {},
				"github.com/maxmcd/bramble/tests/thing": {},
			},
			val:  "github.com/maxmcd/bramble/tests/foo",
			want: "github.com/maxmcd/bramble/tests",
		},
		{
			name: "no match",
			deps: map[string]Dependency{
				"github.com/maxmcd/bramble":             {},
				"github.com/maxmcd/bramble/tests":       {},
				"github.com/maxmcd/bramble/tests/thing": {},
			},
			val:  "github.com/maxmcd/garlic",
			want: "",
		},
		{
			name: "no mystery",
			deps: map[string]Dependency{
				"github.com/maxmcd/bramble": {},
			},
			val:  "github.com/maxmcd/bramble",
			want: "github.com/maxmcd/bramble",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < 10; i++ {
				// Random order of range over a map has affected this test in
				// the past so let's run it a few times.
				cfg := Config{
					Package:      Package{Name: "something"},
					Dependencies: tt.deps,
				}
				require.Equal(t, tt.want, cfg.LoadValueToDependency(tt.val))
			}
		})
	}
}
