package bramblescript

import "fmt"

type ErrIncorrectType struct {
	shouldBe string
	is       string
}

func (eit ErrIncorrectType) Error() string {
	return fmt.Sprintf("incorrect type of %q, should be %q", eit.is, eit.shouldBe)
}
