FROM golang:alpine


RUN apk add build-base linux-headers
WORKDIR /go/src/github.com/maxmcd/bramble
COPY go.sum go.mod ./
RUN go mod download

COPY . .

RUN go install

# FROM alpine

# COPY --from=0 /go/src/github.com/maxmcd/bramble/bramble /bin/bramble
