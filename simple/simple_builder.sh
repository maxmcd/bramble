set -uxe
export PATH="$BRAMBLE_PATH/store/$hermes_seed/bin"
echo $PATH
env
mkdir -p $out
# can add --static
gcc -Xlinker -rpath=$hermes_seed/x86_64-linux-musl/lib \
     -o $out/simple $BRAMBLE_PATH/store/$src/simple.c
$out/simple
