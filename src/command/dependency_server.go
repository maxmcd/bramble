package command

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
	"strings"
	"sync"
	"time"

	"github.com/maxmcd/bramble/pkg/httpx"
	"github.com/pkg/errors"
)

type DepServer struct {
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

func dependencyServerHandler() http.Handler {
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
			// TODO: support more than one project per repo
			bramble, err := newBramble(loc, "")
			if err != nil {
				job.Error = err.Error()
				return
			}

			buildResponse, err := bramble.fullBuild(context.Background(), nil, fullBuildOptions{check: true})
			if err != nil {
				job.Error = err.Error()
				return
			}
			if err := bramble.store.AddDependencyMetadata(
				job.Module,
				bramble.project.Version(),
				loc,
				buildResponse.moduleFunctionMapping(),
			); err != nil {
				job.Error = err.Error()
				return
			}
		}()

		return nil
	})
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

func postJob(url, module, reference string) (err error) {
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
