# Bramble

Bramble is a functional build system inspired by [nix](https://nixos.org/).

**This codebase is in active development and is not stable, complete or secure, proceed with caution.**

### Overview

Bramble is a script runner and build system. You can use it to reliably build libraries and executables and run them on various systems.

Bramble uses [starlark](https://docs.bazel.build/versions/master/skylark/language.html) as its build language. Starlark's syntax is inspired by python.

Let's look at a simple example:

```python
def run_busybox():
    # this call triggers the derivation to calculate and build
    bb = busybox()
    # whoami is now available to run
    print(cmd("whoami", clear_env=True, env={"PATH": bb.out + "/bin"}).output())


def busybox():
    # download the executable, this is the only way you are allowed
    # to use the network during builds
    download = derivation(
        name="busybox_download",
        builder="fetch_url",
        env={
            "url": "https://brmbl.s3.amazonaws.com/busybox-x86_64.tar.gz",
            "hash": "2ae410370b8e9113968ffa6e52f38eea7f17df5f436bd6a69cc41c6ca01541a1",
        },
    )

    # we pass the build_busybox callback here which is used to build this derivation
    return derivation(name="busybox", builder=build_busybox, build_inputs=[download])


def build_busybox():
    os.mkdir("$out/bin")  # using the builtin os module

    # move the busybox executable into the output for this build
    print(cmd("echo ok $busybox_download/busybox-x86_64").output())
    cmd(
        "$busybox_download/busybox-x86_64",
        "cp",
        "$busybox_download/busybox-x86_64",
        "$out/bin/busybox",
    ).wait()

    # extract the available commands from the busybox help text
    commands = []
    commands_started = False
    for line in cmd("$out/bin/busybox").output:
        if commands_started:
            for name in line.strip().split(","):
                name = name.strip()
                if name:
                    commands.append(name)
        if "Currently defined functions" in line:  # the builtin os module
            commands_started = True

    # cd into the output directory so that symlinks are relative
    os.cd("$out/bin")
    cmd("./busybox ln -s busybox ln").wait()

    # link each command so that we can use all the available busybox commands
    for c in commands:
        if c != "ln":
            cmd("./ln -s busybox %s" % c).output()
```

We can save this file as "example.bramble" to our filesystem and run it with `bramble run example:run_busybox`

When we do so the following things happen:

1. The file is parsed and the global function `run_busybox` is found and run.
2. `busybox()` is called, which triggers two calls to `derivation()`
3. Each derivation is not built, instead a serialized version of the derivation is calculated and held in memory.
4. When `bb.cmd()` is called the script knows that the build stage is complete. Before running the function, all dependent derivations are built.
5. During the build stage the busybox executable is downloaded and then built using the `build_busybox()` function.
6. After the build is complete `bb.cmd("whoami")` is run and the output is printed.

In order for this all to work there are a few rules:

- You can't call `cmd()`, `derivation.cmd()`, or use any of the `os` module before derivations are done being called. Calls to the current system might be used as build inputs which would lead to inconsistent behavior in different environments.
- Similarly, using the network within a derivation is disallowed, you must use the `fetch_url` builder to fetch files
- Using `cmd()` and `os` are ok within derivation build functions as these calls will (eventually) be sandboxed.

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
bramble run foo

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
