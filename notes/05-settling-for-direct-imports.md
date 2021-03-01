# Direct imports

I initially wanted directory imports. If you import a directory it parses all the bramble files in that directory and gives you all the globals across all files. Similarly, all globals are available to sibling scripts, so just like Go you can just treat all files in the same directory as the same scope.

Alas, as far as I can tell, the ergonomics of the starlark-go lib don't allow this. I can't figure out how to load globals that are just scoped to a specific subset of the function calls. eg: if a file imports various other modules I only have one "global" place to put functions. Global functions in any module would be made available in all other modules, trivially leading to naming conflicts.

So I think we have to go with direct file imports. Let's see how that would work. If I have the following tree:

```
├── examples
│   ├── imports_scratch.bramble
│   ├── new_structure
│   │   ├── foo.bramble
│   │   ├── main.bramble
│   │   └── main_test.bramble
│   ├── nomad_deploy.bramble
│   ├── run.bramble
│   └── vault.bramble
```

If I'm in `run.bramble` I can import like so:
```python
load("examples", "vault")
# vault is a module with all of vault's globals

load("examples/vault", "foo")
# could also import a specific global from vault


load("examples/new_structure", "foo")
# and another from a nested dir

load("examples", "new_structure")
# what if I try and load a folder? We could have a default.bramble?
```

Which is all fine. I guess the command running syntax is one of the annoying changes.

We previous had `bramble run function_name` which just looked for the function name in the current directories scope. I guess we do that here as well? look for the function name in all files, or you could fall back to the module name like:

```bash
bramble run foo
# vault is a file in the directory
bramble run vault:foo
# examples is the module name
bramble run examples/vault:foo
```

Ok, not bad, kind of "magic" though. Maybe default.bramble would make things easier.
