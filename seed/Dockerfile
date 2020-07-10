FROM alpine:3.11

RUN apk add curl tar

WORKDIR /bramble/
RUN curl -L -O https://github.com/andrewchambers/hpkgs-seeds/raw/master/linux-x86_64-seed.tar.gz
RUN tar xzf linux-x86_64-seed.tar.gz && rm linux-x86_64-seed.tar.gz
RUN cp /lib/ld-musl-x86_64.so.1 /bramble/x86_64-linux-musl/lib
RUN find /bramble -type d -exec chmod u+rw {} \;
RUN tar -czvf /linux-x86_64-seed.tar.gz .
