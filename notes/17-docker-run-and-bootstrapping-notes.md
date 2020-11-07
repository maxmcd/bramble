
when starting, run the command within docker and mount the docker socket
as a volume. ensure that you pass docker environment variables in case the host
is using a remote machine.

then just run the command, the docker subcommands will spawn their own docker containers.
make sure to mount the store and the current project. internet is allowed :|


---

how to bootstrap the linux binary:

could use a fetch_url derivation
how does it work for dev? ah, just copy it from

---

bootstrap git

hardcode a starlark reference to git and ensure that it's on the system
every time?

--

bootstrapping problem

bramble needs to download versions of itself for build. in order to download versions of itself it needs git. in order to get git we need a way to download deps without git. hardcode a starlark file into the binary? how do we track that? just have the output path and ensure that it's the same every time? check for the folder on startup?

once we have that we can just track everything else with git...

the problem with this approach is that we are limited to making git a static build, or we'll need to include an entire build toolchain. this would usually be solved by an available cache, but we would still need to provide the fallback.

alternatively we could ship minimal git clone functionality. this would leave another moving target in the binary...

oh maybe we just build a minimal static git binary using the Go client and ship that. that could be statically pulled (and easily built for various environments)

building and tracking that minimal static binary is basically the same as including it within the binary itself, so maybe we bootstrap by just `git clone` with the binary and then build everything else off of that. 
