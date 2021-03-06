set -e


PATH=$PATH:$patchelf/bin:$busybox/bin

echo $PATH

cp -r $src/* $out
export LD_LIBRARY_PATH=$out/lib

for filename in $out/bin/*; do
    if readlink $filename | grep -q "coreutils"; then
        # https://github.com/NixOS/patchelf/issues/96
        continue
    fi
    patchelf --remove-rpath $filename
    patchelf --set-interpreter $out/lib/ld-linux-x86-64.so.2 \
        --set-rpath $out/lib $filename
done

$out/bin/bash --help
$out/bin/coreutils --help

for filename in $out/libexec/gcc/x86_64-unknown-linux-gnu/8.3.0/*; do
    # ignore liblto
    if echo $filename | grep -q "liblto"; then
        continue
    fi
    patchelf --remove-rpath $filename
    patchelf --set-interpreter $out/lib/ld-linux-x86-64.so.2 \
        --set-rpath $out/lib $filename
done
