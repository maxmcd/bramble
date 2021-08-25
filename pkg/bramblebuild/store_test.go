package bramblebuild

import (
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
