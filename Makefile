

all: seed_run simple_run

seed_run:
	go run main.go seed.bramble.py

simple_run:
	go run main.go simple/simple.bramble.py

seed/linux-x86_64-seed.tar.gz:
	./seed/build.sh


test:
	go test ./...
