package config

import "testing"

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
			cfg := Config{
				Module:       ConfigModule{Name: "something"},
				Dependencies: tt.deps,
			}
			if got := cfg.LoadValueToDependency(tt.val); got != tt.want {
				t.Errorf("Config.LoadValueToDependency() = %v, want %v", got, tt.want)
			}
		})
	}
}
