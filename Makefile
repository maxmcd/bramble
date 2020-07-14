

all: seed_run simple_run

seed_run:
	go run main.go seed.bramble.py

simple_run:
	go run main.go simple/simple.bramble.py

seed/linux-x86_64-seed.tar.gz:
	./seed/build.sh

integration_tests:
	go test github.com/maxmcd/bramble/pkg/bramble -v -run=Integration

test:
	go test ./...
