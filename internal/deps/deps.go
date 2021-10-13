package deps

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
	"strings"
	"sync"
	"time"

	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/chunkedarchive"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/httpx"
	"github.com/pkg/errors"
)

type Client struct {
	bramblePath string
}

func New(bramblePath string) *Client {
	return &Client{bramblePath: bramblePath}
}
func (c *Client) joinBramblePath(v ...string) string {
	return filepath.Join(append([]string{c.bramblePath}, v...)...)
}

func (c *Client) LocalVersions(module string) ([]string, error) {
	path := c.joinBramblePath("var/dependencies/src", module)
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

func (c *Client) PostJob(url, module, reference string) (err error) {
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

func (c *Client) AddDependencyMetadata(module, version, src string, mapping map[string]map[string][]string) (err error) {
	srcs := c.joinBramblePath("var/dependencies/src")
	fileDest := filepath.Join(srcs, module+"@"+version)

	// TODO, should be platform specific
	drvs := c.joinBramblePath("var/dependencies/" + types.Platform())
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

func (client *Client) DependencyServerHandler(buildHandler func(location string) (DependencyBuildResponse, error)) http.Handler {
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
			if err := client.AddDependencyMetadata(job.Module, resp.ProjectVersion, loc, resp.ModuleFunctionOutputs); err != nil {
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
	router.GET("/module/platform/:platform/:name/", func(c httpx.Context) error { return nil })
	router.GET("/module/:name", func(c httpx.Context) error {
		// TODO: Return all matches for cached derivation outputs that we have
		// as well?
		name := c.Params.ByName("name")

		path := filepath.Join(client.bramblePath, "var/dependencies/src", name)
		searchGlob := fmt.Sprintf("%s*", path)
		matches, err := filepath.Glob(searchGlob)
		if err != nil {
			return err
		}
		for i, match := range matches {
			matches[i] = strings.TrimPrefix(match, path+"@")
		}
		return json.NewEncoder(c.ResponseWriter).Encode(matches)
	})
	router.GET("/module/:name/:version/source", func(c httpx.Context) error {
		name := c.Params.ByName("name")
		path := filepath.Join(client.bramblePath, "var/dependencies/src", name)
		return chunkedarchive.StreamArchive(c.ResponseWriter, path)
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
