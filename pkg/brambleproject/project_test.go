package brambleproject

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProject(t *testing.T) {
	{
		p, err := NewProject(".")
		require.NoError(t, err)
		assert.Equal(t, p.config.Module.Name, "github.com/maxmcd/bramble")
	}
	{
		p, err := NewProject("./testdata/project")
		require.NoError(t, err)
		assert.Equal(t, p.config.Module.Name, "testproject")

		require.NoError(t, p.AddURLHashesToLockfile(map[string]string{"foo": "bar"}))
	}
}
