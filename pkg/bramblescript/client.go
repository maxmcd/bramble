package bramblescript

import (
	"path/filepath"

	"go.starlark.net/starlark"
)

type Client struct {
	dir string
}

func NewClient(dir string) *Client {
	dir, err := filepath.Abs(dir)
	if err != nil {
		// TODO
		panic(err)
	}
	return &Client{dir: dir}
}

// StarlarkCmd defines the cmd() starlark function.
func (client Client) StarlarkCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargsList []starlark.Tuple) (v starlark.Value, err error) {
	return newCmd(args, kwargsList, nil, client.dir)
}
