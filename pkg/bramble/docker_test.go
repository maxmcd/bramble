package bramble

import (
	"context"
	"testing"
)

func TestDocker(t *testing.T) {
	b := Bramble{}

	b.Docker(context.Background(), "", map[string]string{}, nil)
}
