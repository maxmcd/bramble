FROM debian:10.5

RUN apt-get update && apt-get install -y build-essential wget

ENV CFLAGS="--static"
ENV CXXFLAGS="--static"
WORKDIR /opt

RUN wget https://github.com/NixOS/patchelf/releases/download/0.12/patchelf-0.12.tar.bz2
RUN tar xf patchelf-0.12.tar.bz2
WORKDIR /opt/patchelf-0.12.20200827.8d3a16e
RUN ./configure
RUN make
RUN make install
WORKDIR /usr/local/bin
RUN tar -czvf patchelf.tar.gz patchelf
