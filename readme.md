# Bramble

Bramble is a functional build system inspired by [nix](https://nixos.org/).

Bramble is currently a work-in-progress. Feel free to read the corresponding blog post for context and background: https://maxmcd.com/posts/lets-build-a-nix-guix

Current Project goals:
 - Easy to use and understand
 - Run nothing as root
 - Provide primitives and tools to create reproducible builds (might with the previous goal)
 - First class support for building docker images
 - Binary relocation/renaming
 - (more to come)

## Core Concepts

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

```
bramble run


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
