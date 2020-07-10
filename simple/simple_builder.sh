set -uxe
export PATH="$hermes_seed/bin"
echo $PATH
env
mkdir -p $out
# can add --static
gcc \
  -L "$hermes_seed/x86_64-linux-musl/lib" \
  -I "$hermes_seed/x86_64-linux-musl/include" \
  -Wl,--rpath="$hermes_seed/x86_64-linux-musl/lib" \
  -Wl,--dynamic-linker="$hermes_seed/x86_64-linux-musl/lib/ld-musl-x86_64.so.1" \
  -o $out/simple $src/simple.c

# test
$out/simple
