
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
