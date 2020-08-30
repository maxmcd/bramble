package bramble

import "testing"

func TestIntegration(t *testing.T) {
	b := Bramble{}

	if err := b.test([]string{"../../tests"}); err != nil {
		t.Error(err)
	}
}
