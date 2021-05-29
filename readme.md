# Bramble

Bramble is work-in-progress a functional build system inspired by [nix](https://nixos.org/).


Shell scripting has been frustrating for some time. While I don't think I'll ever really get the handle of control flow and all the various tricks I think the real issue is the uncertainty. What programs are on this system, what features do they have, how can I use them, etc. I think this is the "works on my machine" problem. If I have something working I still don't know how to get it working elsewhere.

Docker felt like a hint of a solution. At least now if I want something to run somewhere else I can pack it up into a Docker image. This isn't a general purpose solution though, and unless you're using complicated tools the docker build process is not reproducible.

Nix was exciting to discover. All of your dependencies are assembled in a graph. Every dependency lives in a folder that is assigned a hash. Many different versions of software can exist on the same system and you can compose them into whatever combination you want. You can define a build, send it off to someone, and more often than not, it'll work for them too.

Bramble is my attempt at taking it one step further. Here are some if the ideas behind it:

- **Project based**: Every project has a `bramble.toml` and `bramble.lock` file that track dependencies and other metadata needed to build the project reliably.
- **Reproducible**: All builds are assumed to be reproducible. Every build must consistently return the same output given the same input. You can write builds that aren't reproducible but they'll likely break.
- **Sandboxed**: All builds are sandboxed, running software with Bramble will attempt to be sandboxed by default. Builds also take as little input as possible (no args, no environment variables, no network). Some of these might be relaxed as the project evolves, but things will hopefully stay very locked down.
- **Dependencies are repositories**: Dependencies are stored in repositories. You might reference them with `load("github.com/maxmcd/bramble")` in a build file or `bramble build github.com/maxmcd/foo:foo` from the command line. Dependencies are project specific.
- **Content-addressible store**: Build outputs and build inputs are stored in directories that are named with the hash of their contents.
- **Store content relocation**: Build outputs naturally contain references to other build outputs. These locations are usually specific to the system that originally built the software. Build references are rewritten so that outputs can be shared with different systems. These values are also replaced with a known default when hashing so that there is consistency between different build environments.
- **Remote build**: Support for various remote build options, build clusters, and potentially more exotic things like P2P build cache sharing, bittorrent support, etc..
- **Starlark**: The configuration language [starlark](https://github.com/google/starlark-go) is used to define builds.

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
- [ ] [Dynamic Dependencies](./notes/25-dynamic-dependencies.md)
- [ ] [Running Build Outputs](https://github.com/maxmcd/bramble/issues/25)
- [ ] Docker/OCI Container Build Output


