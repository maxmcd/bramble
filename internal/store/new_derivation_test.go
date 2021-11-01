package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/maxmcd/bramble/pkg/test"
)

func TestStore_StoreLocalSources(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
		sources SourceFiles
	}{
		{"bad hierarchy", true, SourceFiles{
			ProjectLocation: "./",
			Location:        "../",
			Files:           []string{"foo"},
		}},
		{"first", false, SourceFiles{
			ProjectLocation: "./",
			Location:        "./",
			Files:           []string{"new_derivation_test.go"},
		}},
		{"second", false, SourceFiles{
			ProjectLocation: "../",
			Location:        "./",
			Files:           []string{"store/new_derivation_test.go"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.sources.ProjectLocation, _ = filepath.Abs(tt.sources.ProjectLocation)
			tt.sources.Location, _ = filepath.Abs(tt.sources.Location)
			store, err := NewStore(test.TmpDir(t))
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.StoreLocalSources(context.Background(), tt.sources)
			if (err != nil) != tt.wantErr {
				t.Errorf("Store.StoreLocalSources() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			_, err = store.StoreLocalSources(context.Background(), tt.sources)
			if (err != nil) != tt.wantErr {
				t.Errorf("Store.StoreLocalSources() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
