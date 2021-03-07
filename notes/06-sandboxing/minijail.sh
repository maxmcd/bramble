set -x
args=(
    -T static # we want minijail to assume a static binary in case the binary can't use LD_PRELOAD
    # -U # new user namespace
    -C /tmp/bramble-chroot-713833583 # our chroot location
    # -N # new cgroup namespace
    # -p # new pid namespace
    --mount-dev # mount a minimal dev
    -u maxm  # set user id
    -e
    -g users # set group id
    # -v # new vfs namespace ???
    -b /home/maxm/bramble/bramble_store_padding/bramble_
    -b /home/maxm/bramble/bramble_store_padding/bramble_/bramble_build_directory121331829 1
    -b /home/maxm/bramble/bramble_store_padding/bramble_/bramble_build_directory463911248 1
    /home/maxm/bramble/bramble_store_padding/bramble_/ieqjuyrfv7lmdb2bur76jgjcrd33j7na/bin/sh
)
mkdir -p /tmp/bramble-chroot-713833583/proc
mkdir -p /tmp/bramble-chroot-713833583/dev
mkdir -p /tmp/bramble-chroot-713833583/var
# use this args hack so that we can comment
sudo strace minijail0 "${args[@]}"


# this doesn't work, seems hung up on directory permissions unless
# I make my homedir readable
# even after that it exists with a "file not found" error that I don't understand

# with strace it seems to be having trouble finding a library:
# ¯\_(ツ)_/¯
#
# stat("/nix/store/gafigwfaimlziam6qhw1m8dz4h952g1n-glibc-2.32-35/lib/x86_64", 0x7ffd90b68830) = -1 ENOENT (No such file or directory)
# openat(AT_FDCWD, "/nix/store/ah4h9wpsz8yvrfnlk7ldm97q131b1di7-libcap-2.46-lib/lib/libc.so.6", O_RDONLY|O_CLOEXEC) = -1 ENOENT (No such file or directory)
# wait4(27918, libminijail[27918]: execve(1) failed: No such file or directory
