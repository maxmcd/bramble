![](./notes/bramble.svg)

# Bramble

- [Bramble](#bramble)
  - [Project Status](#project-status)
    - [Feature Status](#feature-status)
  - [Hello World](#hello-world)

<hr>

Bramble is a work-in-progress functional build system inspired by [nix](https://nixos.org/).

Bramble is a functional build system that intends to be a user-frendly, robust, and reliable way to build software. Here are some if the ideas behind it:

- **Project Based**: Every project has a `bramble.toml` and `bramble.lock` file that track dependencies and other metadata needed to build the project reliably.
- **Reproducible**: All builds are assumed to be reproducible. Every build must consistently return the same output given the same input. You can write builds that aren't reproducible but they'll likely break things.
- **Sandboxed**: All builds are sandboxed, running software with Bramble will attempt to be sandboxed by default. Builds also take as little input as possible (no args, no environment variables, no network). Some of these might be relaxed as the project evolves, but things will hopefully stay very locked down.
- **Dependencies**: Dependencies are stored in repositories. You might reference them with `load("github.com/maxmcd/bramble")` in a build file or `bramble build bitbucket.org/maxm/foo:foo` from the command line. Dependencies are project specific.
- **Content-Addressible Store**: Build outputs and build inputs are stored in directories that are named with the hash of their contents.
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
