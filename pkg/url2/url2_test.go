package url2

import (
	"testing"
)

func TestJoin(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"http://example.com/", "/foo"}, "http://example.com/foo"},
		{[]string{"example.com/", "/foo"}, "example.com/foo"},
		{[]string{"example.com/", "/foo/"}, "example.com/foo/"},
		{[]string{"example.com//////", "/foo/"}, "example.com/foo/"},
		{[]string{"foo://example.com//////", "/foo/"}, "foo://example.com/foo/"},

		// TODO: strange things can happen with weird input, rethink
		{[]string{"????://example.com//////", "/foo/"}, "/foo/????://example.com//////"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := Join(tt.args...); got != tt.want {
				t.Errorf("Join() = %v, want %v", got, tt.want)
			}
		})
	}
}
