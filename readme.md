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
  - [Dependencies](#dependencies)
  - [Config language](#config-language)
    - [Sys module](#sys-module)
    - [Assert module](#assert-module)
    - [Files builtin](#files-builtin)
  - [Derivation](#derivation)
    - [Attributes](#attributes)
    - [Dependencies](#dependencies-1)
  - [Builds](#builds)
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

### Introduction

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

### Dependencies

### Config language


#### Sys module

#### Assert module

#### Files builtin

### Derivation

#### Attributes

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

#### URL Fetcher
#### Git Fetcher


#### The build sandbox
#### Recursive Builds
