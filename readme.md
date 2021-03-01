# Bramble
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fmaxmcd%2Fbramble.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fmaxmcd%2Fbramble?ref=badge_shield)


Bramble is a functional build system inspired by [nix](https://nixos.org/).

Goals:
 -
**This codebase is in active development and is not stable, complete or secure, proceed with caution.**

### Overview

Bramble is a functional build system. You can use it to reliably build libraries and executables and run them on various systems.

Bramble uses [starlark](https://docs.bazel.build/versions/master/skylark/language.html) as its build language. Starlark's syntax is a subset of python.

### Derivations

Bramble uses [starlark](https://docs.bazel.build/versions/master/skylark/language.html) as its configuration language.

Derivations are the core building block of Bramble. They describe how how to build a specific package. Here is a simple derivation that downloads a file:

```python
seed = derivation(
    name="seed",
    builder="fetch_url",
    env={
        "decompress": True,
        "url": "https://github.com/maxmcd/bramble/releases/download/v0.0.1/linux-x86_64-seed.tar.gz",
        "hash": "111005a76fa66c148799a8fb67fb784ac47944fcba791efe7599128bbd5884ac",
    },
)
```

And here's a slightly more complicated one that compiles a simple c program:

```python
load("../seed", "seed")

derivation(
    name="simple",
    env={"seed": seed},
    builder="%s/bin/sh" % seed,
    args=["./simple_builder.sh"],
    sources=["./simple.c", "simple_builder.sh"],
)
```

### CLI

Run
```bash
# run the function "foo" in the file default.bramble
bramble run :foo

# run the function foo in the file main.bramble or ./main/default.bramble
bramble run main:foo

# download the package github.com/maxmcd/bramble and run the function "seed"
# in ./seed/default.bramble
bramble run github.com/maxmcd/bramble/seed:seed
```

Test
```bash
# runs rests in the current directory
bramble test

# run tests in the ./tests directory
bramble test ./tests
```

### Store

Bramble stores build artifacts at `$BRAMBLE_STORE` or at `$HOME/bramble`. The path is padded out to a fixed length so that cached build outputs can be relocated to a different users home folder.

As an example, when I initialize Bramble on my computer the store path is at:
```
/home/maxm/bramble/bramble_store_padding/bramble_
```
If I set `$BRAMBLE_PATH` to `/tmp/bramble` it creates the following store path:
```
/tmp/bramble/bramble_store_padding/bramble_store_
```
Both are 49 characters long.

You can read more about [Strategies for Binary Relocation In Functional Build Systems](https://maxmcd.com/posts/strategies-for-binary-relocation/)


## License
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fmaxmcd%2Fbramble.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Fmaxmcd%2Fbramble?ref=badge_large)