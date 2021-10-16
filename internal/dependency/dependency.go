package dependency

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/maxmcd/bramble/internal/config"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/chunkedarchive"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/httpx"
	"github.com/maxmcd/bramble/v/cmd/go/mvs"
	"github.com/pkg/errors"
	"golang.org/x/mod/semver"
)

type DependencyManager struct {
	dependencyDirectory string
	dc                  *dependencyClient
}

func NewDependencyManager(bramblePath string, cfg config.Config) *DependencyManager {
	return &DependencyManager{dependencyDirectory: bramblePath}
}

func (deps *DependencyManager) joinBramblePath(v ...string) string {
	return filepath.Join(append([]string{deps.dependencyDirectory}, v...)...)
}

func (deps *DependencyManager) localModuleVersions(module string) ([]string, error) {
	path := deps.joinBramblePath("src", module)
	searchGlob := fmt.Sprintf("%s*", path)
	matches, err := filepath.Glob(searchGlob)
	if err != nil {
		return nil, err
	}
	for i, match := range matches {
		matches[i] = strings.TrimPrefix(match, path+"@")
	}
	return matches, nil
}

type Version struct {
	Module  string
	Version string
}

func (m Version) String() string {
	return m.Module + "@" + m.Version
}
func (m Version) mvsVersion() mvs.Version {
	parts := strings.SplitN(m.Version, ".", 2)
	return mvs.Version{Name: m.Module + "@" + parts[0], Version: parts[1]}
}

func versionFromMVSVersion(m mvs.Version) Version {
	loc := strings.LastIndex(m.Name, "@")
	return Version{Version: m.Name[loc+1:] + "." + m.Version, Module: m.Name[:loc]}
}

func sortVersions(vs []Version) {
	sort.Slice(vs, func(i, j int) bool { return vs[i].Module < vs[j].Module })
}

func configVersions(cfg config.Config) (vs []Version) {
	for module, dep := range cfg.Dependencies {
		vs = append(vs, Version{Module: module, Version: dep.Version})
	}
	sortVersions(vs)
	return vs
}

func (deps *DependencyManager) existsLocally(m Version) bool {
	return fileutil.PathExists(deps.localModuleLocation(m))
}

func (deps *DependencyManager) localModuleDependencies(m Version) (vs []Version, err error) {
	cfg, err := config.ReadConfig(filepath.Join(deps.localModuleLocation(m), "bramble.toml"))
	if err != nil {
		return nil, err
	}
	return configVersions(cfg), nil
}

func (deps *DependencyManager) remoteModuleDependencies(ctx context.Context, m Version) (vs []Version, err error) {
	cfg, err := deps.dc.getModuleConfig(ctx, m)
	if err != nil {
		return nil, err
	}
	return configVersions(cfg), nil
}

func (deps *DependencyManager) localModuleLocation(m Version) (path string) {
	return deps.joinBramblePath("src", m.String())
}

func (deps *DependencyManager) allVersions(ctx context.Context, module string) (vs []string, err error) {
	return deps.dc.getModuleVersions(ctx, module)
}

func (deps *DependencyManager) PostJob(url, module, reference string) (err error) {
	jr := JobRequest{Module: module, Reference: reference}
	dc := &dependencyClient{client: &http.Client{}, host: url}
	id, err := dc.postJob(context.Background(), jr)
	if err != nil {
		return err
	}
	for {
		job, err := dc.getJob(context.Background(), id)
		if err != nil {
			return err
		}
		if job.Error != "" {
			return errors.New(job.Error)
		}
		if !job.Emd.IsZero() {
			break
		}
		time.Sleep(time.Second)
	}
	return nil
}

func (deps *DependencyManager) reqs() mvs.Reqs {
	return dependencyManagerReqs{deps: deps}
}

type dependencyManagerReqs struct {
	deps *DependencyManager
}

var _ mvs.Reqs = dependencyManagerReqs{}

func (r dependencyManagerReqs) Required(m mvs.Version) (versions []mvs.Version, err error) {
	v := versionFromMVSVersion(m)
	var vs []Version
	if r.deps.existsLocally(v) {
		vs, err = r.deps.localModuleDependencies(v)
	} else {
		// TODO: tracing
		vs, err = r.deps.remoteModuleDependencies(context.Background(), v)
	}
	if err != nil {
		return nil, err
	}
	for _, v := range vs {
		versions = append(versions, v.mvsVersion())
	}
	return
}
func (r dependencyManagerReqs) Max(v1, v2 string) (o string) {
	switch semver.Compare("v0."+v1, "v0."+v2) {
	case -1:
		return v2
	default:
		return v1
	}
}
func (r dependencyManagerReqs) Upgrade(m mvs.Version) (v mvs.Version, err error) {
	panic("")
	return
}
func (r dependencyManagerReqs) Previous(m mvs.Version) (v mvs.Version, err error) {
	panic("")
	return
}

type dependencyClient struct {
	client *http.Client
	host   string
}

func (dc *dependencyClient) request(ctx context.Context, method, path, contentType string, body io.Reader, resp interface{}) (err error) {
	url := fmt.Sprintf("%s/%s",
		strings.TrimSuffix(dc.host, "/"),
		strings.TrimPrefix(path, "/"),
	)
	return httpx.Request(ctx, dc.client, method, url, contentType, body, resp)
}

func (dc *dependencyClient) postJob(ctx context.Context, job JobRequest) (id string, err error) {
	b, err := json.Marshal(job)
	if err != nil {
		return "", err
	}
	return id, dc.request(ctx,
		http.MethodPost,
		"/job",
		"application/json",
		bytes.NewBuffer(b),
		&id)
}

func (dc *dependencyClient) getJob(ctx context.Context, id string) (job Job, err error) {
	return job, dc.request(ctx,
		http.MethodGet,
		"/job/"+id,
		"application/json",
		nil,
		&job)
}

func (dc *dependencyClient) getModuleVersions(ctx context.Context, name string) (vs []string, err error) {
	return vs, dc.request(ctx,
		http.MethodGet,
		"/module/"+name,
		"application/json",
		nil,
		&vs)
}

func (dc *dependencyClient) getModuleConfig(ctx context.Context, m Version) (cfg config.Config, err error) {
	var buf bytes.Buffer
	var w io.Writer = &buf
	if err := dc.request(ctx,
		http.MethodGet,
		"/module/config/"+m.String(),
		"application/json",
		nil, w); err != nil {
		return cfg, err
	}
	return config.ParseConfig(&buf)
}

type Job struct {
	ID        string
	Start     time.Time
	Emd       time.Time
	Error     string
	Module    string
	Reference string
}

type JobRequest struct {
	// The location of the version control repository.
	Module string
	// Reference is a version control reference. With Git this could be a
	// branch, tag, or commit. This value is optional.
	Reference string
}

func (deps *DependencyManager) addDependencyMetadata(module, version, src string, mapping map[string]map[string][]string) (err error) {
	srcs := deps.joinBramblePath("src")
	fileDest := filepath.Join(srcs, module+"@"+version)

	// TODO, should be platform specific
	drvs := deps.joinBramblePath("" + types.Platform())
	metadataDest := filepath.Join(drvs, module+"@"+version)

	// If the metadata is here we already have a record of the output mapping.
	// If we checked the src directory it might just be there as a dependency of
	// another nomad project
	if fileutil.PathExists(metadataDest) {
		return errors.Errorf("version %s of module %q is already present on this server", version, module)
	}

	if err := os.MkdirAll(fileDest, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(metadataDest), 0755); err != nil {
		return err
	}
	if err := fileutil.CopyDirectory(src, fileDest); err != nil {
		return err
	}

	f, err := os.Create(metadataDest)
	if err != nil {
		return err
	}
	e := json.NewEncoder(f)
	e.SetIndent("", "  ")
	if err := e.Encode(mapping); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

type jobQueue struct {
	jobs map[string]*Job
	lock sync.Mutex
}

func (jq *jobQueue) AddJob(job *Job) {
	jq.lock.Lock()
	defer jq.lock.Unlock()

	for {
		// unique id
		job.ID = fmt.Sprint(rand.Int())
		if _, found := jq.jobs[job.ID]; !found {
			break
		}
	}
	job.Start = time.Now()
	if len(jq.jobs) > 5 {
		jq.kickOldest()
	}
	jq.jobs[job.ID] = job
}

func (jq *jobQueue) Lookup(id string) *Job {
	jq.lock.Lock()
	defer jq.lock.Unlock()
	job := jq.jobs[id]
	if job != nil {
		// make copy, a read only record
		v := *job
		return &v
	}
	return nil
}

func (jq *jobQueue) kickOldest() {
	oldest := time.Now()
	id := ""
	for i, j := range jq.jobs {
		if j.Start.Before(oldest) {
			oldest = j.Start
			id = i
		}
	}
	delete(jq.jobs, id)
}

var jq = &jobQueue{jobs: map[string]*Job{}}

type DependencyBuildResponse struct {
	ModuleFunctionOutputs map[string]map[string][]string
	ProjectVersion        string
}

func (deps *DependencyManager) DependencyServerHandler(buildHandler func(location string) (DependencyBuildResponse, error)) http.Handler {
	router := httpx.New()
	router.GET("/job/:id", func(c httpx.Context) error {
		job := jq.Lookup(c.Params.ByName("id"))
		if job == nil {
			return httpx.ErrNotFound(errors.New("no job found with that id"))
		}
		return json.NewEncoder(c.ResponseWriter).Encode(job)
	})
	router.POST("/job", func(c httpx.Context) error {
		jobRequest := JobRequest{}
		if err := json.NewDecoder(c.Request.Body).Decode(&jobRequest); err != nil {
			return httpx.ErrUnprocessableEntity(err)
		}
		job := &Job{
			Module:    jobRequest.Module,
			Reference: jobRequest.Reference,
		}
		jq.AddJob(job)
		fmt.Fprint(c.ResponseWriter, job.ID)

		// Run job
		go func() {
			var err error
			defer func() {
				job.Emd = time.Now()
				fmt.Println(err)
			}()

			loc, err := downloadGithubRepo(job.Module, job.Reference)
			if err != nil {
				job.Error = errors.Wrap(err, "error downloading git repo").Error()
				return
			}
			resp, err := buildHandler(loc)
			if err != nil {
				job.Error = err.Error()
				return
			}
			if err := deps.addDependencyMetadata(job.Module, resp.ProjectVersion, loc, resp.ModuleFunctionOutputs); err != nil {
				job.Error = err.Error()
				return
			}
		}()

		return nil
	})
	// router.GET("/module/outputs/:platform/:name/:version", func(c httpx.Context) error {
	// 	name := c.Params.ByName("name")
	// 	path := filepath.Join(bramblePath, "var", platform, name)
	// 	searchGlob := fmt.Sprintf("%s*", path)
	// 	matches, err := filepath.Glob(searchGlob)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	for i, match := range matches {
	// 		matches[i] = strings.TrimPrefix(match, path+"@")
	// 	}
	// 	return json.NewEncoder(c.ResponseWriter).Encode(matches)
	// })
	// This is hard because :name can have slashes...
	router.GET("/module/platform/:platform/:name_version/", func(c httpx.Context) error { return nil })
	router.GET("/module/versions/:name", func(c httpx.Context) error {
		// TODO: Return all matches for cached derivation outputs that we have
		// as well?
		name := c.Params.ByName("name")
		matches, err := deps.localModuleVersions(name)
		if err != nil {
			return err
		}
		return json.NewEncoder(c.ResponseWriter).Encode(matches)
	})
	router.GET("/module/source/:name_version", func(c httpx.Context) error {
		name := c.Params.ByName("name_version")
		path := filepath.Join(deps.dependencyDirectory, "src", name)
		return chunkedarchive.StreamArchive(c.ResponseWriter, path)
	})
	router.GET("/module/config/:name_version", func(c httpx.Context) error {
		name := c.Params.ByName("name_version")
		path := filepath.Join(deps.dependencyDirectory, "src", name, "bramble.toml")
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.Copy(c.ResponseWriter, f); err != nil {
			return err
		}
		return nil
	})
	// for fun?
	// router.GET("/module/:name/:version/source.tar", func(c httpx.Context) error
	// router.GET("/module/:name/:version/source.tar.gz", func(c httpx.Context) error
	return router
}

func downloadGithubRepo(url string, reference string) (location string, err error) {
	url = "https://" + url + ".git"
	location, err = os.MkdirTemp("", "")
	if err != nil {
		return
	}

	// TODO: replace all this with calls to bramble
	script := fmt.Sprintf(`
	set -ex
	git clone %s %s
	cd %s`, url, location, location)
	if reference != "" {
		script += fmt.Sprintf("\ngit checkout %s", reference)
	}
	// script += "\nrm -rf ./.git"
	cmd := exec.Command("bash", "-c", script)
	var buf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return "", errors.Wrap(err, buf.String())
	}
	return
}
