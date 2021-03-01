# New simpler cli api

We could get something closer to make's simplicity. Let's say you have a bramble file like so:

```python
def foo():
    cmd("echo foo")

def drv():
    derivation("hello")
```

You would then run `bramble foo` within the directory to run the foo method. This would work for derivations or command runs and would work across files (like Go, globals from separate files are merged).

This would need to be combined with our absolute import strategy.

We can extend this model further:
1
```python
# function names prefixed by `test_` are private and can be run as tests
def test_foo():
    cmd("echo foo")

# a function that starts with an underscore is private
def _drv():
    derivation("hello")
```

## Command Line

We need to be able to:
 - reference the global function name we want to call
 - optionally reference a specific build location/package
 - pass arguments

```bash
bramble run [module]:func [args ...]

# run a specific module
bramble run github.com/maxmcd/bramble/lib/busybox:busybox --help

# run in current directory
bramble run busybox

# run in current directory
bramble run .:busybox

# run in parent directory
bramble run ..:busybox
```

## Derivations

What happens with derivations? Are they just a thing that can be referenced? How do I create a shell? Is there just a func for that, I code it up? Maybe it's better that derivations are just a building block and not just "the" thing? Requiring functions to compose derivations is a little more self-documenting.
