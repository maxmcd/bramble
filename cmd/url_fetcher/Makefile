url_fetcher.tar.gz: go.mod go.sum main.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -tags netgo -ldflags '-w' .
	strip url_fetcher
	tar -czvf url_fetcher.tar.gz url_fetcher

upload: url_fetcher.tar.gz
	env AWS_PROFILE=max aws s3 cp ./url_fetcher.tar.gz s3://brmbl/url_fetcher.tar.gz --acl public-read
