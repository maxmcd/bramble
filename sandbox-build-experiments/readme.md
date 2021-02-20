

1. process running the build
2. process running within the sandbox
3. non-root build process

- bramble spawns a worker process by running a setuid script
- the setuid script runs as root and then spawns a non-root process to run the actual task
- communicates over a socket to run jobs or tell the worker to die


-------

 - start a build
 - calculate all inputs
 - spin up workers numbered `MIN(NUM_PARALLEL, NUM_JOBS)`
 - start listening on socket
 - when a work connects to the socket feed it an available job
 - worker bind mounts to the bramble store (likely using the same path from the host, maybe bad for windows)
 - the worker acts as root or a known specific user (ie: could use root for chroot/namespaces and then drop root).
 - only bind mount is to bramble store, the build orchestrator must copy sources into the bramble store
 - the actual build is then run using a build user
 - once the build is complete the response status is sent back to the parent, and the builder optionally waits for the next job

- for osx we create a sandbox and mount a fuse volume. need to figure out how to set permissions for the user, but hopefully the same as linux
- for docker we just launch a docker container for the workers, unless we're on osx, then we need to run the orchestrator within a container as well
- for linux we chroot and bind mount
