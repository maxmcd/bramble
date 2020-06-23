# Bramble

nix + go + docker

A nix-style build system that:
 - has great cache characteristics
 - is open to all (third party or private packages are first class)
 - has first class support for building docker images
 - uses starlark as a build language
 - maybe uses GOPATH style resource caching

# Content addressability

 https://www.reddit.com/r/NixOS/comments/9rriwn/how_will_making_nixpkgs_content_addressable_make/
https://www.youtube.com/watch?v=8M6yvJC00J4&feature=youtu.be&t=782


# Notes

Source of mkDerivation: https://github.com/NixOS/nixpkgs/blob/master/pkgs/stdenv/generic/make-derivation.nix

Derivation with multiple outputs:

```
cat /nix/store/y4hp8gli1r1xw391lrijdvkch83sbk48-clang-10.0.0.drv | pretty-derivation
  { outputs =
      fromList
        [ ( "lib"
          , DerivationOutput
              { path =
                  FilePath
                    "/nix/store/6gv3x5xxg00lg500li9mcb4wymr39n7a-clang-10.0.0-lib"
              , hashAlgo = ""
              , hash = ""
              }
          )
        , ( "out"
          , DerivationOutput
              { path =
                  FilePath "/nix/store/ys8r7d24s03sz2yxj0989ahv0pdxyx86-clang-10.0.0"
              , hashAlgo = ""
              , hash = ""
              }
          )
        , ( "python"
          , DerivationOutput
              { path =
                  FilePath
                    "/nix/store/633jidi6yymhim9ggssrvvxf03rc3q2z-clang-10.0.0-python"
              , hashAlgo = ""
              , hash = ""
              }
          )
        ]
```

And source: https://github.com/NixOS/nixpkgs/blob/master/pkgs/development/compilers/llvm/7/clang/default.nix

# Imports and dependencies

```python
# loads the file random.bramble
random = load("github.com/maxmcd/brambles", "random")

# now we have access to a bash program
print(random.bash)


# like go, would make sense that we have a minimal "stdlib"
build_python_package, numpy = load("languages/python", "build_python_package", "numpy")
# this would help with package bloat, something people could point at, but would also have
# to be somewhat opinionated.


# imagine also since go relies on "one package per dir" we would either need to come up
# with a single valid filename like bazel's BUILD or switch to pointing to
# exact filenames, like so:
load("languages/python.bramble", "build_python_package")
load("github.com/maxmcd/brambles/random.bramble", "random")

# let's think about how this would work with sibling files:

# pointing at files:
python = load("./python.bramble")
# or
random_string = load("./random.bramble", "string")
random = load("./random.bramble")
print(random.string)

# this is hard now because the api now uses file names. we could use the
# default.nix approach. yeah that could be flexible

# could be loading the ./python/default.bramble
# could be loading ./python.bramble
python = load("./python")
# or
random_string = load("./random.bramble", "string")
random = load("./random.bramble")
print(random.string)
```

# Content hashes

```python
# loads the file random.bramble
random = load("github.com/maxmcd/brambles", "random")
```

TODO


# Derivation

<!-- derivation {
  name = "simple";
  builder = "${bash}/bin/bash";
  args = [ ./simple_builder.sh ];
  inherit gcc coreutils;
  src = ./simple.c;
  system = builtins.currentSystem;
} -->
```python
bash, gcc, coreutils = load("core", ["bash", "gcc", "coreutils"])

build(
    name = "simple",
    builder = "{bash}/bin/bash".format(bash) or bash+"/bin/bash",
    args = "./builder.sh",
    include = [gcc, coreutils],
    src = "./simple.c",
)
```


```python




build = load("build")

ruby = load("languages/ruby")

build(
    runtimeDependencies = [ruby],
    build =
)


def build_script(env):
    files = env.fetch_url(url="", hash="asd")

    env.script("""
    mv {files} ./
    cargo build ./
    """.format(files))


```


## Parsing a derivation

1. Which build are we building in the file? All? A subset?
2. Parse the file, find the builds we want to run and find the imports related to them.
3. If the file has no dependencies then generate a json drv with a name that is a hash of its contents (without output).
4. Check the drv cache, do we have the result of this drv?
5. If we have dependencies then download them (or visit them on the filesystem). Crawl those downloaded dependencies recursively until we have every drv and content resulting from a drv.
6. Create the json output for that drv with all of its source drv's and now build the new output.



# Bootstrapping

https://bellard.org/tcc/

https://guix.gnu.org/blog/2019/guix-reduces-bootstrap-seed-by-50/
https://guix.gnu.org/blog/2020/guix-further-reduces-bootstrap-seed-to-25/
