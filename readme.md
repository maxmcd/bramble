# Bramble

***This codebase is in active development and is not stable, complete or secure, proceed with caution.***

Bramble is a functional build system inspired by [nix](https://nixos.org/). You can use it to reliably build libraries and executables and run them on various systems.

Bramble uses [starlark](https://docs.bazel.build/versions/master/skylark/language.html) as its build language. Starlark maintains the functional guarantees that are needed by bramble but has a friendly syntax that is a subset of python.

### Derivations

Derivations are the core building block of Bramble. They describe how how to build a specific package. Here is a simple derivation that downloads a file:

```python
seed = derivation(
    name="seed",
    builder="fetch_url",
    env={
        "url": "https://github.com/maxmcd/bramble/releases/download/v0.0.1/linux-x86_64-seed.tar.gz",
    },
)
```

And here's a slightly more complicated one that compiles a simple c program:

```python
load(nix_seed="github.com/maxmcd/bramble/lib/nix-seed")

def simple():
    return derivation(
        name="simple",
        builder=nix_seed.stdenv().out + "/bin/bash",
        env={"stdenv": nix_seed.stdenv()},
        args=["./simple_builder.sh"],
        sources=files(["./simple.c", "simple_builder.sh"]),
    )
```

### Using Bramble

#### Basic Setup

The first time you run bramble it will create a directory to store all of the build state at `$HOME/bramble`. When first created that directory looks like this:
```
maxm:~/bramble $ tree .
.
├── bramble_store_padding
│   └── bramble_
├── store -> ./bramble_store_padding/bramble_
├── tmp
└── var
    ├── builds
    ├── config-registry
    └── star-cache
```

*This might look a little different for you since we [pad the bramble store path](#store) out to a fixed length.*

In order to run bramble, you'll need to create a bramble.toml file at the root of your project. For the moment it just needs to contain something like the following:

```toml
[module]
name = "github.com/maxmcd/tutorial"
```

The module name is similar to a module name in a [go.mod](https://blog.golang.org/using-go-modules). If you're writing a library, it should the url of your source. If you're writing a program it can be anything.

#### Our First Bramble File

Let's create a file at the root of our project called `default.bramble`. Within that file, let's add the following:

```python
load("github.com/maxmcd/bramble/lib/std")


def fetch_a_url():
    return std.fetch_url("https://maxmcd.com")
```

Now, from the root of our project, we want to ask bramble to build the derivation returned by `fetch_a_url`. Here are all the valid commands we could run to make that happen:

```
bramble build :fetch_a_url
bramble build ./default:fetch_a_url
bramble build github.com/maxmcd/tutorial:fetch_a_url
bramble build github.com/maxmcd/tutorial/default:fetch_a_url
```

The `default.bramble` file is special and will allow you to just pass a function name. Otherwise you'd need to name the file, like `foo:fetch_a_url` if the file was `foo.bramble`.

When we run that command we get the following output:
```
Building derivation h6fdq5wk3bqua67sua2bq5pdo2ojhda3-.drv
Downloading url https://maxmcd.com/
Output at /home/maxm/bramble/bramble_store_padding/bramble_/2t4snrv2mfoch2opft37kyzg4fh6g5n2
```

If we visit the output path we'll find a file called `maxmcd.com` with the contents of https://maxmcd.com.

Something else has also happened, we now have a `bramble.lock` file with the following contents:
```toml
[URLHashes]
  "https://maxmcd.com/" = "b17315f3eac25dc23a59cc0ec2820c78a0b9890f7fea5a44deaef5a3c6cd9e59"
```

This is a record of the hash of the url contents. If we (or someone using our library) downloads the file again and the contents have changed the build will fail, ensuring repeatability.

#### Our First Derivation

TODO

### CLI

Build
```bash
# build the function "foo" in the file default.bramble
bramble build :foo

# build the function foo in the file main.bramble or ./main/default.bramble
bramble build main:foo

# download the package github.com/maxmcd/bramble and build the function "seed"
# in ./seed/default.bramble
bramble build github.com/maxmcd/bramble/seed:seed
```

### Store

Bramble stores build artifacts at `$BRAMBLE_STORE` or at `$HOME/bramble`. The path is padded out to a fixed length so that cached build outputs can be relocated to a different users home folder.

As an example, when I initialize Bramble on my computer the store path is at:
```
/home/maxm/bramble/bramble_store_padding/bramble_
```
If I set `$BRAMBLE_PATH` to `/tmp/bramble` it creates the following store path:
```
/tmp/bramble/bramble_store_padding/bramble_store_
```
Both are 49 characters long.

You can read more about [Strategies for Binary Relocation In Functional Build Systems](https://maxmcd.com/posts/strategies-for-binary-relocation/)
