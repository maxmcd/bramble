package command

import (
	"bytes"
	"encoding/json"
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

func (d *DepServer) handler() http.Handler {
	router := httpx.New()
	router.POST("/job", func(c httpx.Context) error {
		var job Job
		if err := json.NewDecoder(c.Request.Body).Decode(&job); err != nil {
			return httpx.ErrUnprocessableEntity(err)
		}

		loc, err := d.DownloadGithubRepo(job.Module)
		if err != nil {
			return err
		}
		_ = loc

		bramble, err := newBramble(loc, "")
		if err != nil {
			return err
		}

		_, err = bramble.fullBuild(c.Request.Context(), nil, fullBuildOptions{check: true})
		return err
	})
	return router
}

func (d *DepServer) DownloadGithubRepo(url string) (location string, err error) {
	url = "https://" + url + ".git"
	location, err = os.MkdirTemp("", "")
	if err != nil {
		return
	}
	cmd := exec.Command("git", "clone", url, location)
	var buf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return "", errors.Wrap(err, buf.String())
	}
	return
}
