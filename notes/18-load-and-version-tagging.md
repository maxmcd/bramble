

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
