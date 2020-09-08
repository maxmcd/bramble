set -uxe
export PATH="$seed/bin"
echo $PATH
env
mkdir -p $out
gcc \
  -L "$seed/x86_64-linux-musl/lib" \
  -I "$seed/x86_64-linux-musl/include" \
  -ffile-prefix-map=OLD=NEW \
  -Wl,--rpath="$seed/x86_64-linux-musl/lib" \
  -Wl,--dynamic-linker="$seed/x86_64-linux-musl/lib/ld-musl-x86_64.so.1" \
  -o $out/simple ./simple.c

$out/simple
