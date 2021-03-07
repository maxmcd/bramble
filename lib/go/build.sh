set -ex


export PATH=$stdenv/bin:$busybox/bin
export LD_LIBRARY_PATH=$stdenv/lib

mkdir -p /var/tmp
mkdir /tmp

cp -r $go1_4/go .
include=$(pwd)/go/include
cd ./go/src
ls -lah ./cmd/dist
export GO_CCFLAGS="-L $stdenv/lib -I $include -I $stdenv/include-glibc -I $stdenv/include -Wl,-rpath=$stdenv/lib -Wl,--dynamic-linker=$stdenv/lib/ld-linux-x86-64.so.2 "
export CC="gcc -L $stdenv/lib -I $include -I $stdenv/include-glibc -I $stdenv/include -Wl,-rpath=$stdenv/lib -Wl,--dynamic-linker=$stdenv/lib/ld-linux-x86-64.so.2 "
# export GOROOT_BOOTSTRAP=$(pwd)
export CGO_ENABLED="0"
sed -i 's/set -e/set -ex/g' ./make.bash
# cat ./make.bash
bash ./make.bash
