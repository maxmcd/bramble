Thinking about the alias feature for projects. We could just maintain a `bramble.toml` that is used to configure the user's global commands.

We could have a `$BRAMBLE_PATH/bramble.toml` and when a user did `bramble global alias github.com/maxmcd/busybox ash` we would add the alias to the global file. We would then also need to get `$BRAMBLE_PATH/bin` into a users path, probably, and then add shims to that folder to invoke bramble as that program.

We could also support history of global installs this way, if desired, so that a user can roll back/forward their program versions.

We'd be maintaining a `bramble.lock` for these aliases and tracking their sub-dependencies so that they're pinned at specific versions.

Still need to think about "user global" vs "system global", but there are many more problems there.


