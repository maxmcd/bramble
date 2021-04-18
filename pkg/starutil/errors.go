package starutil

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

type ErrIncorrectType struct {
	shouldBe string
	is       string
}

func (eit ErrIncorrectType) Error() string {
	return fmt.Sprintf("incorrect type of %q, should be %q", eit.is, eit.shouldBe)
}

type ErrUnhashable string

func (err ErrUnhashable) Error() string {
	return fmt.Sprintf("%s is unhashable", string(err))
}

func AnnotateError(err error) string {
	sb := new(strings.Builder)
	if err, ok := errors.Cause(err).(*starlark.EvalError); ok {
		if len(err.CallStack) > 0 && err.CallStack.At(0).Pos.Filename() == "assert.star" {
			err.CallStack.Pop()
		}
		fmt.Fprintln(sb)
		fmt.Fprintf(sb, "error: %s\n", err.Msg)

		fmt.Fprint(sb, callStackString(err.CallStack))
		return sb.String()
	}
	fmt.Fprintf(sb, "%+v\n", err)
	return sb.String()
}

func callStackString(stack starlark.CallStack) string {
	out := new(strings.Builder)
	fmt.Fprintf(out, "traceback (most recent call last):\n")

	for _, fr := range stack {
		fmt.Fprintf(out, "  %s: in %s\n", fr.Pos, fr.Name)
		line := sourceLine(fr.Pos.Filename(), fr.Pos.Line)
		fmt.Fprintf(out, "    %s\n", strings.TrimSpace(line))
	}
	return out.String()
}

func sourceLine(path string, lineNumber int32) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	var index int32 = 1
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return ""
		}
		if index == lineNumber {
			return line
		}
		index++
	}
}
