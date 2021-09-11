set -ex

export LD_LIBRARY_PATH=$stdenv/lib

mkdir -p /var/tmp

cp -r $go1_4/go $out
cd $out
include=$(pwd)/go/include
cd $out/go/src
ls -lah ./cmd/dist
export GO_LDFLAGS="-L $stdenv/lib -I $include -I $stdenv/include-glibc -I $stdenv/include"
export CC="gcc -L $stdenv/lib -I $include -I $stdenv/include-glibc -I $stdenv/include -Wl,-rpath=$stdenv/lib -Wl,--dynamic-linker=$stdenv/lib/ld-linux-x86-64.so.2 "

export CGO_ENABLED="0"
sed -i 's/set -e/set -ex/g' ./make.bash

bash ./make.bash


mkdir $out/bin
cd $out/bin
ln -s ../go/bin/go ./go
ln -s ../go/bin/gofmt ./gofmt

# this works for a few things, but has trouble finding the network, resolve.conf
# syslog, no go in PATH,
# stat testdata/libmach8db: no such file or directory
#  stat /Users/rsc/bin/xed: no such file or directory
# stat /usr/local/bin/arm-linux-elf-objdump: no such file or directory
# ../bin/go test all
