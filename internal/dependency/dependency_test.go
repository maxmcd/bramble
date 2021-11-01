package dependency

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/maxmcd/bramble/internal/config"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/fxt"
	"github.com/maxmcd/bramble/v/cmd/go/mvs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"
)

func module(m string, deps ...string) func() (string, []string) {
	return func() (string, []string) { return m, deps }
}

func testDepMgr(t *testing.T, deps ...func() (string, []string)) *Manager {
	dm := &Manager{dir: dir(t.TempDir())}
	for i, dep := range deps {
		module, deps := dep()
		if err := os.MkdirAll(dm.dir.join("src", module), 0755); err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(module, "@")
		cfg := config.Config{
			Module: config.ConfigModule{
				Name:    parts[0],
				Version: parts[1],
			},
			Dependencies: map[string]config.ConfigDependency{},
		}
		for _, d := range deps {
			parts := strings.Split(d, "@")
			name, version := parts[0], parts[1]
			cfg.Dependencies[name] = config.ConfigDependency{Version: version}
		}
		f, err := os.Create(dm.dir.join("src", module, "bramble.toml"))
		if err != nil {
			t.Fatal(err)
		}
		cfg.Render(f)
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			dm.cfg = cfg
		}
	}

	return dm
}

func blogScenario(t *testing.T) *Manager {
	return testDepMgr(t,
		module("A@1.1.0", "B@1.2.0", "C@1.2.0"),
		module("B@1.1.0", "D@1.1.0"),
		module("B@1.2.0", "D@1.3.0"),
		module("C@1.1.0"),
		module("C@1.2.0", "D@1.4.0"),
		module("C@1.3.0", "F@1.1.0"),
		module("D@1.1.0", "E@1.1.0"),
		module("D@1.2.0", "E@1.1.0"),
		module("D@1.3.0", "E@1.2.0"),
		module("D@1.4.0", "E@1.2.0"),
		module("E@1.1.0"),
		module("E@1.2.0"),
		module("E@1.3.0"),
		module("F@1.1.0", "G@1.1.0"),
		module("G@1.1.0", "F@1.1.0"),
	)
}

func TestDMReqsRequired(t *testing.T) {
	dm := blogScenario(t)
	reqs := dm.reqs()
	deps, err := reqs.Required(mvs.Version{
		Name:    "A@1",
		Version: "1.0",
	})
	require.Equal(t, deps, []mvs.Version{
		{Name: "B@1", Version: "2.0"},
		{Name: "C@1", Version: "2.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDMReqs(t *testing.T) {
	dm := blogScenario(t)
	vs, err := mvs.BuildList(mvs.Version{Name: "A@1", Version: "1.0"}, dm.reqs())
	if err != nil {
		t.Fatal(err)
	}
	// https://research.swtch.com/vgo-mvs
	require.Equal(t, vs, []mvs.Version{
		{"A@1", "1.0"},
		{"B@1", "2.0"},
		{"C@1", "2.0"},
		{"D@1", "4.0"},
		{"E@1", "2.0"},
	})
}

func TestSV(t *testing.T) {
	fmt.Println(semver.Compare("v0.3.0", "v0.4.0"))
}

func TestDMReqsUpgrade(t *testing.T) {
	dm := blogScenario(t)

	// Patch local A@1.1.0 to have new version of C before we upgrade
	dm.cfg.Dependencies["C"] = config.ConfigDependency{Version: "1.3.0"}
	vs, err := mvs.Upgrade(
		mvs.Version{Name: "A@1", Version: "1.0"},
		dm.reqs(),
		mvs.Version{Name: "C@1", Version: "3.0"},
	)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, vs, []mvs.Version{
		{"A@1", "1.0"},
		{"B@1", "2.0"},
		{"C@1", "3.0"},
		{"D@1", "3.0"},
		{"E@1", "2.0"},
		{"F@1", "1.0"},
		{"G@1", "1.0"},
	})
}

func (dm *Manager) deleteHalfDeps(t *testing.T) {
	list, err := filepath.Glob(dm.dir.join("src", "*"))
	if err != nil {
		t.Fatal(err)
	}
	rand.Shuffle(len(list), func(i, j int) { list[i], list[j] = list[j], list[i] })
	half := list[:len(list)/2]
	for _, p := range half {
		if err := os.RemoveAll(p); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDMReqsRemote(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			rand.Seed(int64(i))

			remoteDM := blogScenario(t)
			localDM := blogScenario(t)

			// Delete half of the dependencies in the local DM to simulate a
			// partially present subset
			localDM.deleteHalfDeps(t)

			server := httptest.NewServer(ServerHandler(string(remoteDM.dir), nil, nil))

			localDM.dependencyClient = &dependencyClient{
				client: &http.Client{},
				host:   server.URL,
			}

			vs, err := mvs.BuildList(mvs.Version{Name: "A@1", Version: "1.0"}, localDM.reqs())
			if err != nil {
				t.Fatal(err)
			}

			// https://research.swtch.com/vgo-mvs
			require.Equal(t, []mvs.Version{
				{"A@1", "1.0"},
				{"B@1", "2.0"},
				{"C@1", "2.0"},
				{"D@1", "4.0"},
				{"E@1", "2.0"},
			}, vs)
		})
	}
}

func TestDMPathOrDownload(t *testing.T) {
	remoteDM := blogScenario(t)
	localDM := testDepMgr(t) // no deps

	server := httptest.NewServer(ServerHandler(string(remoteDM.dir), nil, nil))

	localDM.dependencyClient = &dependencyClient{
		client: &http.Client{},
		host:   server.URL,
	}

	path, err := localDM.ModulePathOrDownload(context.Background(), Version{"A", "1.1.0"})
	if err != nil {
		fxt.Printpvln(err)
		t.Fatal(err)
	}
	cfg, err := config.ReadConfig(filepath.Join(path, "bramble.toml"))
	if err != nil {
		t.Fatal(err)
	}
	// This is strange, since we just happen to know that "A@1.1.0" is going to
	// be the default config for remoteDM. We might want to fetch the "A@1.1.0"
	// config more directly in the future
	require.Equal(t, cfg, remoteDM.cfg)
}

func TestVersion_mvsVersion(t *testing.T) {
	tests := []struct {
		name string
		have Version
		want mvs.Version
	}{
		{
			name: "simple",
			have: Version{
				Module:  "github.com/maxmcd/bramble",
				Version: "0.1.0",
			},
			want: mvs.Version{
				Name:    "github.com/maxmcd/bramble@0",
				Version: "1.0",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.have.mvsVersion(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Version.mvsVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_versionFromMVSVersion(t *testing.T) {
	tests := []struct {
		name string
		have mvs.Version
		want Version
	}{
		{
			name: "simple",
			have: mvs.Version{
				Name:    "github.com/maxmcd/bramble@0",
				Version: "1.0",
			},
			want: Version{
				Module:  "github.com/maxmcd/bramble",
				Version: "0.1.0",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionFromMVSVersion(tt.have); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("versionFromMVSVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

type testBuilder struct {
	modules  map[string]types.Module
	t        *testing.T
	location string
}

var (
	_ types.NewBuilder = new(testBuilder).NewBuilder
	_ types.Builder    = new(testBuilder)
)

func (tb *testBuilder) NewBuilder(location string) (types.Builder, error) {
	tb.location = location
	return tb, nil
}

func (tb *testBuilder) Modules() map[string]types.Module {
	out := map[string]types.Module{}
	// Make paths absolute
	for loc, m := range tb.modules {
		out[filepath.Join(tb.location, loc)] = m
	}
	return out
}

func (tb *testBuilder) Build(ctx context.Context, location string, args []string, opts types.BuildOptions) (resp types.BuildResponse, err error) {
	return
}

func (tb testBuilder) testGithubDownloader(url, reference string) (location string, err error) {
	location = tb.t.TempDir()
	for loc, m := range tb.modules {
		_ = os.MkdirAll(filepath.Join(location, loc), 0755)
		f, err := os.Create(filepath.Join(location, loc, "/bramble.toml"))
		if err != nil {
			return "", err
		}
		cfg := config.Config{
			Module: config.ConfigModule{
				Name:    m.Name,
				Version: m.Version,
			},
		}
		cfg.Render(f)
		if err := f.Close(); err != nil {
			return "", err
		}
	}
	return location, nil
}

func TestPushJob(t *testing.T) {
	tb := testBuilder{
		t: t,
		modules: map[string]types.Module{
			"": {
				Name:    "x.y/z",
				Version: "2.0.0",
			},
			"./a": {
				Name:    "x.y/z/a",
				Version: "1.2.0",
			},
		},
	}

	server := httptest.NewServer(
		serverHandler(t.TempDir(), tb.NewBuilder, tb.testGithubDownloader),
	)

	if err := PostJob(server.URL, "x.y/z", ""); err != nil {
		t.Fatal(err)
	}
	dc := &dependencyClient{
		host:   server.URL,
		client: &http.Client{},
	}
	for _, m := range tb.modules {
		{
			cfg, err := dc.getModuleConfig(context.Background(), Version{Module: m.Name, Version: m.Version})
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, cfg.Module.Name, m.Name)
			assert.Equal(t, cfg.Module.Version, m.Version)
		}
		{
			body, err := dc.getModuleSource(context.Background(), Version{Module: m.Name, Version: m.Version})
			if err != nil {
				t.Fatal(err)
			}
			_ = body
		}
	}
}
