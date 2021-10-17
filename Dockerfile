FROM golang:alpine


RUN apk add build-base linux-headers
WORKDIR /go/src/github.com/maxmcd/bramble
COPY go.sum go.mod ./
RUN go mod download

COPY . .

RUN go install

FROM alpine
RUN apk add bash git
COPY --from=0 /go/bin/bramble /bin/bramble

CMD ["/bin/bramble", "server", "--host=0.0.0.0", "--port=8080"]
