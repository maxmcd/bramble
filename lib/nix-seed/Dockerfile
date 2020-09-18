FROM debian

RUN apt-get update
RUN apt-get install -y build-essential patchelf wget

WORKDIR /opt

RUN wget http://tarballs.nixos.org/stdenv-linux/x86_64/c5aabb0d603e2c1ea05f5a93b3be82437f5ebf31/bootstrap-tools.tar.xz
RUN tar xf bootstrap-tools.tar.xz
