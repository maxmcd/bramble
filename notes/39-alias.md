Like direnv, set up a bash hook and load aliases into the path as needed. Could add aliases to `bramble.toml` as follows:

```toml
[aliases]
"go" = "github.com/maxmcd/bramble/lib:go"
"python" = {package="github.com/maxmcd/bramble/lib:go", read_only_paths=["./"]}
```

You're allowed to add this anywhere, we'll have to switch to a mode where a project is defined by a `bramble.toml` that has a `[package]` section. By default programs are sandbox to only access files within a project. This can be overridden with attributes as seen above.


If we did this people might want to use `bramble.toml` like this without a module. We'd need to have a lockfile. Maybe we construct this like cargo where a workspace must be defined. Complicates things though...
