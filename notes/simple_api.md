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

```python
# function names prefixed by `test_` are private and can be run as tests
def test_foo():
    cmd("echo foo")

# a function that starts with an underscore is private
def _drv():
    derivation("hello")
```
