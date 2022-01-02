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
	"github.com/maxmcd/bramble/internal/netcache"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/fxt"
	"github.com/maxmcd/bramble/v/cmd/go/mvs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"
)

func pkg(m string, deps ...string) func() (string, []string) {
	return func() (string, []string) { return m, deps }
}

func testDepMgr(t *testing.T, deps ...func() (string, []string)) (config.Config, *Manager) {
	dm := &Manager{dependencyDirectory: dependencyDirectory(t.TempDir())}
	var returnedConfig config.Config
	for i, dep := range deps {
		pkg, deps := dep()
		if err := os.MkdirAll(dm.dependencyDirectory.join("src", pkg), 0755); err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(pkg, "@")
		cfg := config.Config{
			Package: config.Package{
				Name:    parts[0],
				Version: parts[1],
			},
			Dependencies: map[string]config.Dependency{},
		}
		for _, d := range deps {
			parts := strings.Split(d, "@")
			name, version := parts[0], parts[1]
			cfg.Dependencies[name] = config.Dependency{Version: version}
		}
		f, err := os.Create(dm.dependencyDirectory.join("src", pkg, "bramble.toml"))
		if err != nil {
			t.Fatal(err)
		}
		cfg.Render(f)
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			returnedConfig = cfg
		}
	}
	return returnedConfig, dm
}

func blogScenario(t *testing.T) (config.Config, *Manager) {
	return testDepMgr(t,
		pkg("A@1.1.0", "B@1.2.0", "C@1.2.0"),
		pkg("B@1.1.0", "D@1.1.0"),
		pkg("B@1.2.0", "D@1.3.0"),
		pkg("C@1.1.0"),
		pkg("C@1.2.0", "D@1.4.0"),
		pkg("C@1.3.0", "F@1.1.0"),
		pkg("D@1.1.0", "E@1.1.0"),
		pkg("D@1.2.0", "E@1.1.0"),
		pkg("D@1.3.0", "E@1.2.0"),
		pkg("D@1.4.0", "E@1.2.0"),
		pkg("E@1.1.0"),
		pkg("E@1.2.0"),
		pkg("E@1.3.0"),
		pkg("F@1.1.0", "G@1.1.0"),
		pkg("G@1.1.0", "F@1.1.0"),
	)
}

func TestDMReqsRequired(t *testing.T) {
	cfg, dm := blogScenario(t)
	reqs := dm.reqs(cfg)
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
	cfg, dm := blogScenario(t)
	vs, err := mvs.BuildList(mvs.Version{Name: "A@1", Version: "1.0"}, dm.reqs(cfg))
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
	cfg, dm := blogScenario(t)

	// Patch local A@1.1.0 to have new version of C before we upgrade
	cfg.Dependencies["C"] = config.Dependency{Version: "1.3.0"}

	vs, err := mvs.Upgrade(
		mvs.Version{Name: "A@1", Version: "1.0"},
		dm.reqs(cfg),
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
	list, err := filepath.Glob(dm.dependencyDirectory.join("src", "*"))
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

			_, remoteDM := blogScenario(t)
			cfg, localDM := blogScenario(t)

			// Delete half of the dependencies in the local DM to simulate a
			// partially present subset
			localDM.deleteHalfDeps(t)

			server := httptest.NewServer(ServerHandler(string(remoteDM.dependencyDirectory), nil, nil))

			localDM.dependencyClient = &dependencyClient{
				client:      &http.Client{},
				host:        server.URL,
				cacheClient: netcache.NewStdCache(server.URL),
			}

			vs, err := mvs.BuildList(mvs.Version{Name: "A@1", Version: "1.0"}, localDM.reqs(cfg))
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
	remoteCFG, remoteDM := blogScenario(t)
	_, localDM := testDepMgr(t) // no deps

	server := httptest.NewServer(ServerHandler(string(remoteDM.dependencyDirectory), nil, nil))

	localDM.dependencyClient = &dependencyClient{
		client:      &http.Client{},
		host:        server.URL,
		cacheClient: netcache.NewStdCache(server.URL),
	}

	path, err := localDM.PackagePathOrDownload(context.Background(), types.Package{"A", "1.1.0"})
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
	require.Equal(t, cfg, remoteCFG)
}

func TestVersion_mvsVersionFromPackage(t *testing.T) {
	tests := []struct {
		name string
		have types.Package
		want mvs.Version
	}{
		{
			name: "simple",
			have: types.Package{
				Name:    "github.com/maxmcd/bramble",
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
			if got := mvsVersionFromPackage(tt.have); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Version.mvsVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_packageFromMVSVersion(t *testing.T) {
	tests := []struct {
		name string
		have mvs.Version
		want types.Package
	}{
		{
			name: "simple",
			have: mvs.Version{
				Name:    "github.com/maxmcd/bramble@0",
				Version: "1.0",
			},
			want: types.Package{
				Name:    "github.com/maxmcd/bramble",
				Version: "0.1.0",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := packageFromMVSVersion(tt.have); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("packageFromMVSVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

type testBuilder struct {
	packages map[string]types.Package
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

func (tb *testBuilder) Packages() map[string]types.Package {
	out := map[string]types.Package{}
	// Make paths absolute
	for loc, m := range tb.packages {
		out[filepath.Join(tb.location, loc)] = m
	}
	return out
}

func (tb *testBuilder) Build(ctx context.Context, location string, args []string, opts types.BuildOptions) (resp types.BuildResponse, err error) {
	return
}

func (tb testBuilder) testGithubDownloader(url, reference string) (location string, err error) {
	location = tb.t.TempDir()
	for loc, m := range tb.packages {
		_ = os.MkdirAll(filepath.Join(location, loc), 0755)
		f, err := os.Create(filepath.Join(location, loc, "/bramble.toml"))
		if err != nil {
			return "", err
		}
		cfg := config.Config{
			Package: config.Package{
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
		packages: map[string]types.Package{
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

	if err := PostJob(context.Background(), server.URL, "x.y/z", ""); err != nil {
		t.Fatal(err)
	}

	dc := &dependencyClient{
		host:        server.URL,
		client:      &http.Client{},
		cacheClient: netcache.NewStdCache(server.URL),
	}
	for _, m := range tb.packages {
		{
			cfg, err := dc.getPackageConfig(context.Background(), types.Package{Name: m.Name, Version: m.Version})
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, cfg.Config.Package.Name, m.Name)
			assert.Equal(t, cfg.Config.Package.Version, m.Version)
		}
		{
			loc := t.TempDir()
			err := dc.getPackageSource(context.Background(), types.Package{Name: m.Name, Version: m.Version}, loc)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
