FROM goreleaser/goreleaser

RUN apk add gcc linux-headers \
    && ln -s /usr/bin/x86_64-alpine-linux-musl-gcc /bin/musl-gcc
