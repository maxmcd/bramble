

Within a `load()` statement:

1. Relative paths are not allowed
2. Must point to module name not function
3. Can point to external modules as long as they exist in the dependencies

In the cli:

1. `bramble build` can only build things in this project, only takes relative paths to modules, `./...` statements, or module names prefixed with this projects name. (what about subprojects?). We'll also use the go requirement and required a preceeding `./` in order for module file paths to be used.
2. `bramble run` can run things in this project, can run external things that are in the dependency file, and will add a dependency to a project if `bramble run` triggers a search for an external dependency. (we likely need to add `indirect` addtribute to dependencies)
3. `bramble run` can also run external projects outside of a project, will run a specific version or fetch the latest version


Test cases:
- `bramble build ./...` => ok
- `bramble build tests` => not ok
- `bramble build github.com/maxmcd/bramble/...` => ok
- `bramble build ./lib` => ok if there is lib/default.bramble
- `bramble build ./internal` => not ok since there is no ./internal/default.bramble
- `bramble build :all` => not ok, no relative path
- `bramble build ./:all` => ok if there is ./default.bramble and a function `all`
- `bramble build github.com/maxmcd/bramble/tests/...` => ok
- `bramble build github.com/maxmcd/busybox/...` => not ok

- `bramble run ./...` => not ok, can't run many things
- `bramble run tests` => not ok, not relative path
- `bramble run :print_simple` => not ok, no relative path
- `bramble run ./:print_simple simple` => ok if there is ./default.bramble and a function `all`
- `bramble run ./lib:git git` => ok if there is lib/default.bramble with function git
- `bramble run ./internal:foo foo` => not ok since there is no ./internal/default.bramble
- `bramble run github.com/maxmcd/bramble:print_simple simple` => ok
- `bramble run github.com/maxmcd/busybox:busybox ash` => ok, will run version in project or add latest, if outside of project will run latest
- `bramble run github.com/maxmcd/busybox@0.0.1:busybox ash` => same as above but will use specific version
