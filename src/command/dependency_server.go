package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"

	"github.com/maxmcd/bramble/pkg/httpx"
	"github.com/pkg/errors"
)

type DepServer struct {
}

type Job struct {
	// The location of the version control repository.
	Module string
	// Reference is a version control reference. With Git this could be a
	// branch, tag, or commit. This value is optional.
	Reference string
}

func dependencyServerHandler() http.Handler {
	router := httpx.New()
	router.POST("/job", func(c httpx.Context) error {
		var job Job
		if err := json.NewDecoder(c.Request.Body).Decode(&job); err != nil {
			return httpx.ErrUnprocessableEntity(err)
		}

		loc, err := downloadGithubRepo(job.Module, job.Reference)
		if err != nil {
			return errors.Wrap(err, "error downloading git repo")
		}
		// TODO: support more than one project per repo
		bramble, err := newBramble(loc, "")
		if err != nil {
			return err
		}

		buildResponse, err := bramble.fullBuild(c.Request.Context(), nil, fullBuildOptions{check: true})
		if err != nil {
			return err
		}
		if err := bramble.store.AddDependencyMetadata(
			job.Module,
			bramble.project.Version(),
			loc,
			buildResponse.moduleFunctionMapping(),
		); err != nil {
			return err
		}

		return err
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
