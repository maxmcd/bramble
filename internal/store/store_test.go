package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ensureBramblePath(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Error(err)
	}
	// -1 because the output path is "/foo/bar" plus the trailing slash
	assert.Equal(t, len(s.StorePath), PathPaddingLength-1)

	_ = os.MkdirAll("/tmp/bramble-test-34079652", 0755)

}

func Test_calculatePaddedDirectoryNameAll(t *testing.T) {
	start := "/"
	for i := 0; i < PathPaddingLength-4; i++ {
		start += "b"
		t.Run(start, func(t *testing.T) {
			out, err := calculatePaddedDirectoryName(start, PathPaddingLength)
			if err != nil {
				t.Fatal(err)
			}
			// fullpath includes a trailing slash because it would always have
			// one when replacing a store reference in a build output
			fullPath := filepath.Join(start, out) + "/"
			if len(fullPath) != PathPaddingLength {
				t.Errorf("len of %s is not %d it's %d", fullPath, PathPaddingLength, len(fullPath))
			}
		})
	}
}

func Test_calculatePaddedDirectoryName(t *testing.T) {
	type args struct {
		bramblePath   string
		paddingLength int
	}
	tests := []struct {
		name        string
		args        args
		errContains string
	}{{
		name: "shorter",
		args: args{
			bramblePath:   "/bramble",
			paddingLength: 15,
		},
	}, {
		name: "basic",
		args: args{
			bramblePath:   "/bramble",
			paddingLength: 40,
		},
	}, {
		name: "basic",
		args: args{
			bramblePath:   "/tmp/bramble-test-34079652",
			paddingLength: PathPaddingLength,
		},
	}, {
		name: "basic",
		args: args{
			bramblePath:   "/bramble",
			paddingLength: 20,
		},
	}, {
		name: "shortest possible",
		args: args{
			bramblePath:   "/bramble",
			paddingLength: 11,
		},
	}, {
		name: "too short",
		args: args{
			bramblePath:   "/bramble",
			paddingLength: 10,
		},
		errContains: "path that is too long",
	}, {
		name: "too short",
		args: args{
			bramblePath:   "/bramble",
			paddingLength: 9,
		},
		errContains: "path that is too long",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculatePaddedDirectoryName(tt.args.bramblePath, tt.args.paddingLength)
			if err != nil {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("calculatePaddedDirectoryName() got error %q but wanted error to contain %q", err, tt.errContains)
				}
				return
			}
			fullPath := filepath.Join(tt.args.bramblePath, got) + "/"
			if len(fullPath) != tt.args.paddingLength {
				t.Errorf("calculatePaddedDirectoryName() output path %q is len '%v' should be %d", fullPath, len(fullPath), tt.args.paddingLength)
			}
		})
	}
}
