package assert

import (
	"fmt"
	"regexp"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

var assertModule = `# Predeclared built-ins for this module:
#
# error(msg): report an error in Go's test framework without halting execution.
#  This is distinct from the built-in fail function, which halts execution.
# catch(f): evaluate f() and returns its evaluation error message, if any
# matches(str, pattern): report whether str matches regular expression pattern.
# module(**kwargs): a constructor for a module.
# _freeze(x): freeze the value x and everything reachable from it.
#
# Clients may use these functions to define their own testing abstractions.

def _eq(x, y):
    if x != y:
        error("%r != %r" % (x, y))

def _ne(x, y):
    if x == y:
        error("%r == %r" % (x, y))

def _true(cond, msg = "assertion failed"):
    if not cond:
        error(msg)

def _lt(x, y):
    if not (x < y):
        error("%s is not less than %s" % (x, y))

def _contains(x, y):
    if y not in x:
        error("%s does not contain %s" % (x, y))

def _fails(f, pattern):
    "assert_fails asserts that evaluation of f() fails with the specified error."
    msg = catch(f)
    if msg == None:
        error("evaluation succeeded unexpectedly (want error matching %r)" % pattern)
    elif not matches(pattern, msg):
        error("regular expression (%s) did not match error (%s)" % (pattern, msg))

freeze = _freeze  # an exported global whose value is the built-in freeze function

assert = module(
    "assert",
    fail = error,
    eq = _eq,
    ne = _ne,
    true = _true,
    lt = _lt,
    contains = _contains,
    fails = _fails,
)`

// Copyright 2017 The Bazel Authors. All rights reserved.

const localKey = "Reporter"

// A Reporter is a value to which errors may be reported.
// It is satisfied by *testing.T.
type Reporter interface {
	Error(err error)
	FailNow() bool
}

// SetReporter associates an error reporter (such as a testing.T in
// a Go test) with the Starlark thread so that Starlark programs may
// report errors to it.
func SetReporter(thread *starlark.Thread, r Reporter) {
	thread.SetLocal(localKey, r)
}

// GetReporter returns the Starlark thread's error reporter.
// It must be preceded by a call to SetReporter.
func GetReporter(thread *starlark.Thread) Reporter {
	r, ok := thread.Local(localKey).(Reporter)
	if !ok {
		panic("internal error: starlarktest.SetReporter was not called")
	}
	return r
}

var (
	once      sync.Once
	assert    starlark.StringDict
	assertErr error
)

// LoadAssertModule loads the assert module.
// It is concurrency-safe and idempotent.
func LoadAssertModule() (starlark.StringDict, error) {
	once.Do(func() {
		predeclared := starlark.StringDict{
			"error":   starlark.NewBuiltin("error", Error),
			"catch":   starlark.NewBuiltin("catch", catch),
			"matches": starlark.NewBuiltin("matches", matches),
			"module":  starlark.NewBuiltin("module", starlarkstruct.MakeModule),
			"_freeze": starlark.NewBuiltin("freeze", freeze),
		}
		thread := new(starlark.Thread)
		assert, assertErr = starlark.ExecFile(thread, "assert.star", assertModule, predeclared)
	})
	return assert, assertErr
}

// catch(f) evaluates f() and returns its evaluation error message
// if it failed or None if it succeeded.
func catch(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var fn starlark.Callable
	if err := starlark.UnpackArgs("catch", args, kwargs, "fn", &fn); err != nil {
		return nil, err
	}
	if _, err := starlark.Call(thread, fn, nil, nil); err != nil {
		return starlark.String(err.Error()), nil
	}
	return starlark.None, nil
}

// matches(pattern, str) reports whether string str matches the regular expression pattern.
func matches(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var pattern, str string
	if err := starlark.UnpackArgs("matches", args, kwargs, "pattern", &pattern, "str", &str); err != nil {
		return nil, err
	}
	ok, err := regexp.MatchString(pattern, str)
	if err != nil {
		return nil, fmt.Errorf("matches: %s", err)
	}
	return starlark.Bool(ok), nil
}

// error(x) reports an error to the Go test framework.
func Error(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("error: got %d arguments, want 1", len(args))
	}
	stk := thread.CallStack()
	stk.Pop()
	err := &starlark.EvalError{
		CallStack: stk,
	}
	if s, ok := starlark.AsString(args[0]); ok {
		err.Msg = s
	} else {
		err.Msg = args[0].String()
	}
	reporter := GetReporter(thread)
	reporter.Error(err)
	if reporter.FailNow() {
		return starlark.None, err
	}
	return starlark.None, nil
}

// freeze(x) freezes its operand.
func freeze(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(kwargs) > 0 {
		return nil, fmt.Errorf("freeze does not accept keyword arguments")
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("freeze got %d arguments, wants 1", len(args))
	}
	args[0].Freeze()
	return args[0], nil
}
