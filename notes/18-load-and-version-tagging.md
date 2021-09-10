

```python
load("github.com/maxmcd/bramble", "master")
load("github.com/maxmcd/bramble", "dev")
load("github.com/maxmcd/bramble", "2174408")
load("github.com/maxmcd/bramble", "v0.4.5")
```

Allow a version to be passed to the load statement. This would help provide support for different versions of the same dependency.

Open questions:
- How does this look in bramble.toml?
- How is the "default" tag determined?



---------------


Why not just allow the user to override the major version in the dependency file and alias it to another name.

Like:

```go
require (
	github.com/BurntSushi/toml v0.4.1 alias github.com/BurntSushi/toml/v2
	github.com/bmatcuk/doublestar/v4 v4.0.2
```

It's strange because it might allow for too much flexibiity. Confusion about what imports actually are if they're significantly changed when aliasing.

Go's versioning scheme looks good in this regard. Allowing people to only pin the major version of a dependencey pushes people to not have major changes without a major version bump.

Maybe though the version of the bramble package will follow the software that it ships. Programs might need a very specific version. maybe better to invest in software that helps people test merging different dependencies.

So maybe we just want the specific `@branch_or_version` on the imports within the files themselves. Also allows more expectation of configuring by writing code and not writing configuration.
