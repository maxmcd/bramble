set -uxe
export PATH="$hermes_seed/bin"
echo $PATH
env
mkdir -p $out
# can add --static
gcc -Xlinker -rpath=$hermes_seed/x86_64-linux-musl/lib \
     -o $out/simple $src/simple.c
$out/simple
