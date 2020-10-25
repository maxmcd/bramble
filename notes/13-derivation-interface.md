# Derivation function api

```python

# file builder with env vars and args
derivation(
    name="foo", # optional
    builder="%s/bin/sh" % d,
    env={"PATH": "{}/bin/:{}/bin".format(d, e)},
    args=["-c", "ls -lah {}".format(f)],
)


derivation(
    name="foo", # optional
    builder=function,
    args=[f, g],
)




```
