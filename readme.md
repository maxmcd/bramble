# Bramble

nix + go + docker

A nix-style build system that:
 - has great cache characteristics
 - is open to all (third party or private packages are first class)
 - has first class support for building docker images
 - uses starlark as a build language
 - maybe uses GOPATH style resource caching


# Imports and dependencies

```python
# loads the file random.bramble
random = load("github.com/maxmcd/brambles", "random")

# now we have access to a bash program
print(random.bash)


# like go, would make sense that we have a minimal "stdlib"
build_python_package, numpy = load("languages/python", "build_python_package", "numpy")
# this would help with package bloat, something people could point at, but would also have
# to be somewhat opinionated.


# imagine also since go relies on "one package per dir" we would either need to come up
# with a single valid filename like bazel's BUILD or switch to pointing to
# exact filenames, like so:
load("languages/python.bramble", "build_python_package")
load("github.com/maxmcd/brambles/random.bramble", "random")

# let's think about how this would work with sibling files:

# pointing at files:
python = load("./python.bramble")
# or
random_string = load("./random.bramble", "string")
random = load("./random.bramble")
print(random.string)

# this is hard now because the api now uses file names. we could use the
# default.nix approach. yeah that could be flexible

# could be loading the ./python/default.bramble
# could be loading ./python.bramble
python = load("./python")
# or
random_string = load("./random.bramble", "string")
random = load("./random.bramble")
print(random.string)
```

# Content hashes

```python
# loads the file random.bramble
random = load("github.com/maxmcd/brambles", "random")
```
