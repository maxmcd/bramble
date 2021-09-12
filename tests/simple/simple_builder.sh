set -uxe

export PATH="$stdenv/bin"

mkdir -p $out/bin
gcc \
  -L "$stdenv/lib" \
  -I "$stdenv/include" \
  -ffile-prefix-map=OLD=NEW \
  -Wl,--rpath="$stdenv/lib" \
  -Wl,--dynamic-linker="$stdenv/lib/ld-linux-x86-64.so.2" \
  -o $out/bin/simple ./simple.c

$out/bin/simple

patchelf --print-interpreter --print-soname --print-rpath $out/bin/simple
