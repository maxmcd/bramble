# Sandboxing

The original intent was to have bramble be rootless. I think this is still an admirable goal, but in the immediate term it conflicts with some security issues.

The module import and command running syntax will allow this:
```
bramble run github.com/maxmcd/unsafe/virus:inject
```
If bramble is going to have this level of flexibility it should be sandboxed by default.

## Links and ideas

- https://blog.bazel.build/2017/08/25/introducing-sandboxfs.html - FUSE driver to prevent from copying files into a chroot every time, https://blog.bazel.build/2018/04/13/preliminary-sandboxfs-support.html
- https://blog.bazel.build/2015/09/11/sandboxing.html - bazel sandboxing description
- https://www.infoq.com/articles/build-a-container-golang/ - basic container stuff in golang, likely not useful
- https://unix.stackexchange.com/questions/79692/running-program-as-root-without-using-sudo-with-normal-user-account - setuid
- https://superuser.com/questions/290903/how-to-run-a-program-as-root-without-sudo
- https://chromium.googlesource.com/chromiumos/docs/+/master/sandboxing.md - chromium os sandboxing guidelines
- https://github.com/netblue30/firejail - open source jail binary
- https://github.com/google/minijail - google's jail binary
- https://github.com/andrewchambers/hermes/blob/master/doc/man/hermes-package-store.7.md - hermes using multiple build users (why?)
- https://nixos.org/manual/nix/stable/#ssec-multi-user - same with nix
- https://github.com/NixOS/nix/issues/1429 - nix disallows setuid
- https://bazel.build/designs/2016/06/02/sandboxing.html
Support docker as an option, would work only for linux or just for running things in docker.
- https://unix.stackexchange.com/questions/6433/how-to-jail-a-process-without-being-root interesting notes


## Linux

- Default mounts: https://github.com/opencontainers/runtime-tools/blob/a7974a4078764ec41acf5feaa05f07854af44aa6/generate/generate.go#L174-L211
- Create dev/null https://www.commandlinefu.com/commands/view/24199/create-devnull-if-accidentally-deleted-or-for-a-chroot
- container linux with mknod examples https://github.com/cloudify-incubator/cloudify-rest-go-client/blob/f8139d8e38b0909fae3e4212eb05497483c0e5b8/container/container_linux.go
- https://proot-me.github.io/ - (quite a performance hit)
- https://github.com/proot-me/proot
- https://unix.stackexchange.com/questions/68956/block-network-access-of-a-process
- https://rootlesscontaine.rs/
- https://www.sciencedirect.com/science/article/pii/S0010465518302042 - good overview
- https://events.static.linuxfound.org/sites/events/files/slides/rootless-containers-2016.pdf
- https://pkg.go.dev/github.com/opencontainers/runc


### Runc

- non-root uid mapping https://github.com/opencontainers/runc/issues/1800


