![](./notes/animated.svg)

<h1 align="center">Bramble</h1>

- [Project Status](#project-status)
  - [Feature Status](#feature-status)
- [Installation](#installation)
  - [Linux](#linux)
- [Hello World](#hello-world)
- [Spec](#spec)
  - [Introduction](#introduction)
  - [Project configuration](#project-configuration)
    - [Module metadata](#module-metadata)
    - [bramble.lock](#bramblelock)
  - [Command Line](#command-line)
    - [`bramble build`](#bramble-build)
    - [`bramble run`](#bramble-run)
    - [`bramble ls`](#bramble-ls)
    - [`bramble repl`](#bramble-repl)
    - [`bramble shell`](#bramble-shell)
    - [`bramble gc`](#bramble-gc)
  - [Dependencies](#dependencies)
  - [Config language](#config-language)
    - [.bramble, default.bramble and the load() statement](#bramble-defaultbramble-and-the-load-statement)
    - [derivation()](#derivation)
    - [run()](#run)
    - [test()](#test)
    - [Sys module](#sys-module)
    - [Assert module](#assert-module)
    - [Files builtin](#files-builtin)
    - [Dependencies](#dependencies-1)
  - [Builds](#builds)
    - [Derivations that output derivations](#derivations-that-output-derivations)
    - [URL Fetcher](#url-fetcher)
    - [Git Fetcher](#git-fetcher)
    - [The build sandbox](#the-build-sandbox)
    - [Recursive Builds](#recursive-builds)
<hr>

Bramble is a work-in-progress functional build system inspired by [nix](https://nixos.org/).

Bramble is a functional build system that intends to be a user-friendly, robust, and reliable way to build software. Here are some if the ideas behind it:

- **Project Based**: Every project has a `bramble.toml` and `bramble.lock` file that track dependencies and other metadata needed to build the project reliably.
- **Reproducible**: All builds are assumed to be reproducible. Every build must consistently return the same output given the same input. You can write builds that aren't reproducible but they'll likely break things.
- **Sandboxed**: All builds are sandboxed, running software with Bramble will attempt to be sandboxed by default. Builds also take as little input as possible (no args, no environment variables, no network). Some of these might be relaxed as the project evolves, but things will hopefully stay very locked down.
- **Dependencies**: Dependencies are stored in repositories. You might reference them with `load("github.com/maxmcd/bramble")` in a build file or `bramble build bitbucket.org/maxm/foo:foo` from the command line. Dependencies are project specific.
- **Content-Addressable Store**: Build outputs and build inputs are stored in directories that are named with the hash of their contents.
- **Store Content Relocation**: Build outputs naturally contain references to other build outputs. These locations are usually specific to the system that originally built the software. Build references are rewritten so that outputs can be shared with different systems. These values are also replaced with a known default when hashing so that there is consistency between different build environments when possible.
- **Remote Build**: Future support for various remote build options, build clusters, and potentially more exotic things like P2P build cache sharing, bittorrent support, etc..
- **Starlark**: The configuration language [starlark](https://github.com/google/starlark-go) is used to define builds.
- **Diverse Build Environment Support**: Will have first class support for all major operating systems and potentially even support for the browser, webassembly, FAAS, and others. (Bramble is Linux-only at the moment).

## Project Status

Many things are broken, would not expect this to work or be useful yet. Things are still quite messy, but the list of features seems to be solidifying so hopefully things will be more organized soon. If you have ideas or would like to contribute please open an issue.

### Feature Status

- [x] Basic Build Functionality
- [x] Store Content Relocation
- [ ] Sandboxing
    - [x] Linux
    - [ ] OSX
- [ ] "Fetch" Build Rules
    - [x] Fetch URL
    - [ ] Fetch Git Repo
- [ ] Remote Dependencies
- [ ] Remote Builds
- [ ] Recursive Builds
- [ ] Documentation Generation
- [ ] [Dynamic Dependencies](./notes/25-dynamic-dependencies.md)
- [ ] [Running Build Outputs](https://github.com/maxmcd/bramble/issues/25)
- [ ] Docker/OCI Container Build Output

## Installation

Install with `go get github.com/maxmcd/bramble` or download a recent binary release.

### Linux

In order for rootless/userspace sandboxing to work "User Namespaces" must be compiled and enabled in your kernel:

- Confirm `CONFIG_USER_NS=y` is set in your kernel configuration (normally found in `/proc/config.gz`)
  ```bash
  $ cat /proc/config.gz | gzip -d | grep CONFIG_USER_NS
  CONFIG_USER_NS=y
  ```
- Arch/Debian: `echo 1 > /proc/sys/kernel/unprivileged_userns_clone`
- RHEL/CentOS 7: `echo 28633 > /proc/sys/user/max_user_namespaces`

## Hello World

Here's an example project that downloads busybox and uses it to create a script that says "Hello world!".

**./bramble.toml**
```toml
[module]
name = "github.com/maxmcd/bramble"
```

**./bramble.lock**
```toml
[URLHashes]
  "https://brmbl.s3.amazonaws.com/busybox-x86_64.tar.gz" = "2ae410370b8e9113968ffa6e52f38eea7f17df5f436bd6a69cc41c6ca01541a1"
```

**./default.bramble**
```python
def fetch_url(url):
    """
    fetch_url is a handy wrapper around the built-in fetch_url builder. It just
    takes the url you want to fetch.
    """
    return derivation(name="fetch-url", builder="fetch_url", env={"url": url})


def fetch_busybox():
    return fetch_url("https://brmbl.s3.amazonaws.com/busybox-x86_64.tar.gz")


def busybox():
    """
    busybox downloads the busybox binary and copies it to an output directory.
    Symlinks are then created for every command busybox supports.
    """
    return derivation(
        name="busybox",
        builder=fetch_busybox().out + "/busybox-x86_64",
        args=["sh", "./script.sh"],
        sources=files(["./script.sh"]),
        env={"busybox_download": fetch_busybox()},
    )


def hello_world():
    bb = busybox()
    PATH = "{}/bin".format(bb.out)

    return derivation(
        "say_hello_world",
        builder=bb.out + "/bin/sh",
        env=dict(PATH=PATH, busybox=bb.out),
        args=[
            "-c",
            """set -e

        mkdir -p $out/bin
        touch $out/bin/say-hello-world
        chmod +x $out/bin/say-hello-world

        echo "#!$busybox/bin/sh" > $out/bin/say-hello-world
        echo "$busybox/bin/echo Hello World!" >> $out/bin/say-hello-world

        $out/bin/say-hello-world
        """,
        ],
    )
```

**./script.sh**
```bash
set -e
$busybox_download/busybox-x86_64 mkdir $out/bin
$busybox_download/busybox-x86_64 cp $busybox_download/busybox-x86_64 $out/bin/busybox
cd $out/bin
for command in $(./busybox --list); do
	./busybox ln -s busybox $command
done
```

If you copy these files into a directory you can build it like so:
```
$ bramble build default:hello_world
bramble path directory doesn't exist, creating
Building derivation q3rl3odtuv2xjiqm3omthbwowddjb5mg-fetch-url.drv
Downloading url https://brmbl.s3.amazonaws.com/busybox-x86_64.tar.gz
Output at /home/maxm/bramble/bramble_store_padding/bramble_/rb2rveatcti4szdt3s6xc37cpvqxrdmr
Building derivation sk4pwh7ci6xyslaeykv35obdvylx6aoy-busybox.drv
Output at  /home/maxm/bramble/bramble_store_padding/bramble_/2xqocntumrj4vp6buucoma2q6a6dfvmf
Building derivation kurlc3txkvnmuumhl3tt3ez45xuhnoc2-say_hello_world.drv
Hello World!
Output at  /home/maxm/bramble/bramble_store_padding/bramble_/okrlinp5whzqkbmhkrddol5jvdzgftp3
```

And run the script:
```
$ /home/maxm/bramble/bramble_store_padding/bramble_/okrlinp5whzqkbmhkrddol5jvdzgftp3/bin/say-hello-world
Hello World!
```

That's it! Your first bramble build.


## Spec

This is a reference manual for Bramble. Bramble is a work-in-progress. I started writing this spec to solidify the major design decisions, but everything is still very much in flux. There are scattered notes in the [notes folder](./notes) as well.

### Introduction

Bramble is a functional build system and package manager. Bramble is project-based, when you run a build or run a build output it must always be done in the context of a project.

Here are three example use-cases that Bramble hopes to support and support well.

1. **Running a build or a command related to a project**. Often, code repositories want to explain how to build or run their software. Bramble aims to be one of the safest and most reliable ways to do that. A `bramble build` or `bramble run` within a project will use the `bramble.toml`, `bramble.lock` and source files to fetch dependencies from a cache or build them from source. Additionally `bramble run` commands are sandboxed by default, so Bramble should be a good choice to run unfamiliar software.
2. **Running arbitrary software from the internet**. Running `bramble run github.com/username/project:function binary` will pull down software from that repo, build it, and run it within a sandbox on a local system. `bramble run` is sandboxed by default and aims to provide a safe and reproducible way to run arbitrary software on your system.
3. **Build a Docker/OCI container**. Any `bramble run` call can be packaged up into a container containing only the bare-minimum dependencies for that program to run.
4. **Future use-cases**. Support for WASM build environments, support for running builds in a browser. Tight integration with IDEs.

### Project configuration

Every Project has a `bramble.toml` file that includes configuration information and a `bramble.lock` file that includes hashes and other metadata that are used to ensure that the project can be built reproducibly.

#### Module metadata

```toml
[module]
name = "github.com/maxmcd/bramble"
```

A project must include a module name. If it's expected that this project is going to be importable as a module then the module name must match the location of the repository where the module is stored.

#### bramble.lock

```toml
[URLHashes]
  "https://brmbl.s3.amazonaws.com/busybox-x86_64.tar.gz" = "2ae410370b8e9113968ffa6e52f38eea7f17df5f436bd6a69cc41c6ca01541a1"
```

The `bramble.lock` file stores hashes so that "fetch" builders like "fetch_url" and "fetch_git" can ensure the contents they are downloading have the expected content. This file will also include various hashes to ensure dependencies and sub-dependencies can be reliably re-assembled.

### Command Line

#### `bramble build`

```
bramble build [options] [module]:<function>
bramble build [options] <path>
```

The `build` command is used to build derivations returned by bramble functions. Calling `build` with a module location and function will call that function, take any derivations that are returned, and build that derivation and its dependencies.

Here are some examples:
```
bramble build ./tests/basic:self_reference
bramble build github.com/maxmcd/bramble:all
bramble build github.com/username/repo/subdirectory:all
```

Calls to `build` with a path argument will build everything in that directory and all of its subdirectories. This is done by searching for all bramble files and calling all of their public functions. Any derivations that are returned by these functions are built along with all of their dependencies (TODO: should a call with a path just search that location). Call to `build` without a path will run all builds from the current directory and its subdirectories.

```
bramble build
bramble build ./tests
```

#### `bramble run`

```
bramble run [options] [module]:<function> [args...]
```

#### `bramble ls`

```
bramble ls <path>
```

Calls to `ls` will search the current directory for bramble files and print their public functions with documentation. If an immediate subdirectory has a `default.bramble` documentation will be printed for those functions as well.

#### `bramble repl`

`repl` opens up a read-eval-print-loop for interacting with the bramble [config language](#config-language). You can make derivations and call other built-in functions. The repl has limited use because you can't build anything that you create, but it's a good place to get familiar with how the built-in modules and functions work.

#### `bramble shell`

`shell` takes the same arguments as `bramble build` but instead of building the final derivation it opens up a terminal into the build environment within a build directory with environment variables and dependencies populated. This is a good way to debug a derivation that you're building.

#### `bramble gc`

`gc` searches for all known projects (TODO: link to what "known projects" means), runs all of their public functions and calculates what derivations and configuration they need to run. All other information is deleted from the store and project configurations.

### Dependencies

### Config language

Bramble uses [starlark](https://github.com/google/starlark-go) for its configuration language. Starlark generally a superset of Python, but has some differences that might trip up more experienced Python users. When in doubt would be sure to check out the [lamnguage spec](https://github.com/google/starlark-go/blob/master/doc/spec.md).

Here is a typical bramble file:

```python
# Load statements
load("github.com/maxmcd/bramble/lib/stdenv")
load("github.com/maxmcd/bramble/lib")
load("github.com/maxmcd/bramble/lib/std")


def fetch_a_url():
    return std.fetch_url("https://maxmcd.com/")


def step_1():
    bash = "%s/bin/bash" % stdenv.stdenv()
    # A derivation, the basic building block of our builds
    return derivation(
        "step_1",
        builder=bash,
        env=dict(bash=bash),
        # Use of the `files()` builtin
        sources=files(["./step1.sh"]),
        args=["./step1.sh"],
    )
```

#### .bramble, default.bramble and the load() statement

Bramble source files are stored in files with a `.bramble` file extension. Files can reference other bramble files by using their module names. This project has the module name `github.com/maxmcd/bramble` so if I want to access a file at `./tests/basic.bramble` I can import it with `load("github.com/maxmcd/bramble/tests/basic")`. Relative imports aren't supported.

The `default.bramble` filename is special. If a directory has a `default.bramble` in it then we can import that directory as a package and all functions in the `default.bramble`. In the above example, if the file was called `default.bramble` instead of `basic.bramble` we could import it with `load("github.com/maxmcd/bramble/tests")`.

If you call `load("github.com/maxmcd/bramble/tests")` at the top of a bramble file a new global variable named `tests` will be loaded into the program context. `tests` will have an attribute for all global variables in the `default.bramble` file unless they begin with an underscore.

```python
# ./tests/default.bramble
def foo():
  print("hi")
def _bar():
  print("hello")
```

```python
# In a `bramble repl`
>>> load("github.com/maxmcd/bramble/tests")
>>> tests
<module "github.com/maxmcd/bramble/tests">
>>> dir(tests)
["foo"]
>>> tests.foo()
hi
>>> tests._bar()
Traceback (most recent call last):
  <stdin>:1:6: in <expr>
Error: module has no ._bar field or method
```

#### derivation()

```python
derivation(name, builder, args=[], sources=[], env={}, outputs=["out"], platform=sys.platform)
```

Derivations are the basic building block of a bramble build. Every build is a graph of derivations. Everything that is built has a derivation and has dependencies that are derivations.

A derivation `name` is required to help with visibility and debugging. It's helpful to have derivation names be unique in a project, but this is not an enforced requirement.

A `builder` can be on of the default built-ins: `["fetch_url", "fetch_git", "derivation_output"]` or it can point to an executable that will be used to build files.


`args` are the arguments that as passed to the builder. `env` defines environment variables that are set within the build environment. Bramble detects dependencies by scanning for derivations referenced within a derivation. `builder`, `args` and `env` are the only parameters that can reference other derivations, so you can be sure that if a derivation isn't referenced in one of those parameters that it won't be available to the build.

`sources` contains references to source files needed for this derivation. Use the `files` builtin to populate sources for a given derivation.

`outputs` defines this Derivation's outputs. The default is for a derivation to have a single output called "out", but you can have one or more output with any name. After a derivation is created, you can reference it's outputs as attributes. If you cast a derivation to a string it returns a reference to the default output.

```python
>>> b = derivation("hi", "ho")
>>> b
{{ soen6obfffrahna6ojyc2cgxjx7jcmhv:out }}
>>> b.out
"{{ soen6obfffrahna6ojyc2cgxjx7jcmhv:out }}"
>>> c = derivation("hi", "ho", outputs=["a", "b", "c"])
>>> c
{{ lvliebpnk6lcalc3sdsvfbrzwlamb4qo:a }}
>>> c.a
"{{ lvliebpnk6lcalc3sdsvfbrzwlamb4qo:a }}"
>>> c.b
"{{ lvliebpnk6lcalc3sdsvfbrzwlamb4qo:b }}"
>>> c.c
"{{ lvliebpnk6lcalc3sdsvfbrzwlamb4qo:c }}"
>>> "{}/bin/bash".format(c)
"{{ lvliebpnk6lcalc3sdsvfbrzwlamb4qo:a }}/bin/bash"
>>> "{}/bin/bash".format(c.b)
"{{ lvliebpnk6lcalc3sdsvfbrzwlamb4qo:b }}/bin/bash"
```

`platform` denotes what platform this derivation can be built on. If the specific platform is available on the current system the derivation will be built.

#### run()

The run function defines the attributes for running a program from a derivation output. If a call to a bramble function returns a run command that run command and parameters will be executed.

```python
run(derivation, args=[], paths=[], write_paths=[], hidden_paths=[], network=False)
```

#### test()

The test command creates a test. Any call to the test function will register a test that can be run later. Calls to `bramble test` will run all tests in that directory and it's children. Calls to a specific bramble function like `bramble test ./tests:first` will run any test functions that are called during the function call.

```python
test(derivation, args=[])
```


#### Sys module

```python
>>> sys
<module "sys">
>>> dir(sys)
["arch", "os", "platform"]
>>> sys.arch
"amd64"
>>> sys.os
"linux"
>>> sys.platform
"linux-amd64"
```


#### Assert module

```python
>>> dir(assert)
["contains", "eq", "fail", "fails", "lt", "ne", "true"]
>>> assert.contains("hihihi", "hi")
>>> assert.contains("hihihi", "how")
Traceback (most recent call last):
  <stdin>:1:16: in <expr>
  assert.star:30:14: in _contains
Error: hihihi does not contain how
```

#### Files builtin

```python
files(include, exclude=[], exclude_directories=True, allow_empty=True)
```

`files` searches for source files and returns a mutable list.


#### Dependencies

### Builds

Bramble builds all derivations within a sandbox. There are OS-specific sandboxes that try and provide similar functionality.

A tree of derivations is assembled to build. The tree is walked, compiling dependencies first, until all derivations are built. If a derivation has already been built (TODO: or is available in a remote store) it is skipped.

When building a specific derivation the steps are as follows:

1. Create a build directory. These are stored in the store (TODO: why?). They typically look something like this `/home/maxm/bramble/bramble_store_padding/bramble_/bramble_build_directory941760171`.
2. Copy any file sources needed for this build into the build directory.
3. Create folders for each output. They look something like this: `/home/maxm/bramble/bramble_store_padding/bramble_/bramble_build_directory451318742/`.
4. If the derivation has a "fetch" builder then that specific builder is run to fetch files using the variables that have been passed.
5. If the regular builder is used the derivation has to be prepared to be built. Paths in the derivation will reference a fixed known store path `/home/bramble/bramble/bramble_store_padding/bramb/`, so we must replace it with the store path (of equal length) used in this system.
6. Once the derivation is ready to build the `builder`, `args`, and `env` attributes are taken and used to run a sandbox. The `builder` program is run and `args` are passed to that program. `env` values are loaded as environment variables in alphabetical order.
7. The output folder locations are loaded by name into the environment variables as well. The value `$out` might have value `/home/maxm/bramble/bramble_store_padding/bramble_/bramble_build_directory451318742/`.
8. The bramble store is mounted to the sandbox so that the build can access any store values that it needs for a build. All store outputs are read-only, but the build directory and all the outputs directories can be written to. (TODO: should we block access to other store directories?)
9. If the build exits with a non-zero exit code it's assumed that the build has failed.
10. Once the build is complete all output directories are hashed so that they can be placed in a folder that is a hash of their contents. Outputs are also searched for any references to dependencies so that the runtime dependencies can be noted. The hashing steps are as follows.
   1. The build output is tarred up into an archive.
   2. The mod times and user ids are stripped from the archive.
   3. References to the build directory are replaced with with a fixed known value so the random number isn't injected into builds. Files from this folder are discarded at the end of the build, so it's ok if we break references.
   4. The archive is copied for hashing. The copy is scanned for this system's store path and replaced with the reference store path `/home/bramble/bramble/bramble_store_padding/bramb/`. This helps ensure outputs hash the same on different systems.
   5. References to the output path in the copy are replaced with null bytes.
   6. The copy is hashed and a folder is created with the hash as the name.
   7. References to the output folder name are replaced with the hash name.
   8. The original archive is expanded into the hash-name folder.
11. The build output hash is added to the derivation (along with all dependency output hashes) before being written to disk.
12. The output folder locations and final derivation are returned.

#### Derivations that output derivations

The `derivation_output` derivation outputs a new derivation graph. This graph will be merged with the existing build graph and the build will continue. There are two rules with this builder:

1. No recursive `derivation_output`. If a derivation uses the builder `derivation_output` it must not output any derivations that use that builder. This will likely be supported in the future but is currently disallowed out of caution.
2. A `derivation_output` must only have the default output "out" and the updated derivation graph must also only have a single outputted derivation that has a single default output. When `derivation_output` is built it replaces a node in the build graph with a new graph. Any references to that old node must be overwritten with references to the new output derivation. In order to ensure that replacement is trivial we must ensure that the old node and the new node have identical output structure.

When a `derivation_output` is called the resulting derivation graph is written to `bramble.lock` so that the output is not rebuilt on other systems.

#### URL Fetcher
#### Git Fetcher


#### The build sandbox
#### Recursive Builds
