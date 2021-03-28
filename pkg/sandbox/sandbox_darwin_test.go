// +build darwin

package sandbox

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSandbox_runCommand(t *testing.T) {
	t.Run("no network", func(t *testing.T) {
		var buf bytes.Buffer
		sbx := Sandbox{
			Stdout:         os.Stdout,
			Stderr:         &buf,
			DisableNetwork: true,
			Path:           "/sbin/ping",
			Args:           []string{"-c1", "google.com"},
			Dir:            "/Users/maxm/go/src/github.com/maxmcd/bramble", //TODO
		}
		require.Error(t, sbx.Run(context.Background()))
		assert.Contains(t, buf.String(), "Unknown host")
	})
	t.Run("yes network", func(t *testing.T) {
		var buf bytes.Buffer
		sbx := Sandbox{
			Stdout:         &buf,
			Stderr:         os.Stderr,
			DisableNetwork: false,
			Path:           "/sbin/ping",
			Args:           []string{"-c1", "google.com"},
			Dir:            "/Users/maxm/go/src/github.com/maxmcd/bramble", //TODO
		}
		require.NoError(t, sbx.Run(context.Background()))
		assert.Contains(t, buf.String(), "0.0% packet loss")
	})
	t.Run("hllo wrld", func(t *testing.T) {
		var buf bytes.Buffer
		sbx := Sandbox{
			Stdout:         &buf,
			Stderr:         os.Stderr,
			DisableNetwork: true,
			Path:           "/bin/echo",
			Args:           []string{"hi"},
			Dir:            "/Users/maxm/go/src/github.com/maxmcd/bramble", //TODO
		}
		require.NoError(t, sbx.Run(context.Background()))
		assert.Equal(t, buf.String(), "hi\n")
	})
}
