

build:
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -tags netgo -ldflags '-w' -o sbx
	sudo chown root:root sbx
	sudo chmod u+s,g+s sbx

run: build
	./sbx


run_server: build
	go run ./server
