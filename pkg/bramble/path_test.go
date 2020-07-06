package bramble

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ensureBramblePath(t *testing.T) {
	_, storePath, err := ensureBramblePath()
	if err != nil {
		t.Error(err)
	}
	// -1 because the output path is "/foo/bar" plus the trailing slash
	assert.Equal(t, len(storePath), PaddingLength-1)
}

func Test_calculatePaddedDirectoryName(t *testing.T) {
	type args struct {
		bramblePath   string
		paddingLength int
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
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
		wantErr: ErrPathTooLong,
	}, {
		name: "too short",
		args: args{
			bramblePath:   "/bramble",
			paddingLength: 9,
		},
		wantErr: ErrPathTooLong,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculatePaddedDirectoryName(tt.args.bramblePath, tt.args.paddingLength)
			if err != nil {
				if err != tt.wantErr {
					t.Errorf("calculatePaddedDirectoryName() got error %q but wanted error %v", err, tt.wantErr)
				}
				return
			}
			fullPath := filepath.Join(tt.args.bramblePath, got) + "/"
			fmt.Println(fullPath)
			if len(fullPath) != tt.args.paddingLength {
				t.Errorf("calculatePaddedDirectoryName() output path %q is len '%v' should be %d", fullPath, len(fullPath), tt.args.paddingLength)
			}
		})
	}
}
