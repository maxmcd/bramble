package fileutil

import (
	"fmt"
	"testing"
)

func TestPathWithinDir(t *testing.T) {
	tests := []struct {
		dir     string
		path    string
		wantErr bool
	}{
		{"./", "./", true},
		{"/home/noexist", "/home", true},
		{"/home/noexist", "../", true},
		{"/home/noexist", "/home/", true},
		{"/home/noexist", "/home/noexist", false},
		{"/home/noexist", "/home/noexist/something", false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.dir+"&"+tt.path), func(t *testing.T) {
			if err := PathWithinDir(tt.dir, tt.path); (err != nil) != tt.wantErr {
				t.Errorf("PathWithinDir() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
