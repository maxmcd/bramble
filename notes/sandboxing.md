# Sandboxing

The original intent was to have bramble be rootless. I think this is still an admirable goal, but in the immediate term it conflicts with some security issues.

The module import and command running syntax will allow this:
```
bramble run github.com/maxmcd/unsafe/virus:inject
```
If bramble is going to have this level of flexibility it should be sandboxed by default.

## Links and ideas

- https://blog.bazel.build/2017/08/25/introducing-sandboxfs.html
- https://www.infoq.com/articles/build-a-container-golang/
- https://unix.stackexchange.com/questions/79692/running-program-as-root-without-using-sudo-with-normal-user-account
- https://superuser.com/questions/290903/how-to-run-a-program-as-root-without-sudo
Support docker as an option, would work only for linux or just for running things in docker.
