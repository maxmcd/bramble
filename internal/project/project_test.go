package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProject(t *testing.T) {
	{
		p, err := NewProject(".")
		require.NoError(t, err)
		assert.Equal(t, p.config.Package.Name, "github.com/maxmcd/bramble")
	}
	{
		p, err := NewProject("./testdata/project")
		require.NoError(t, err)
		assert.Equal(t, p.config.Package.Name, "testproject")
		writer := p.LockfileWriter()
		if err := writer.AddEntry("foo", "bar"); err != nil {
			t.Fatal(err)
		}
		if err := p.WriteLockfile(); err != nil {
			t.Fatal(err)
		}

	}
}
