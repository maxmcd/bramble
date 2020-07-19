

all: seed_run simple_run

seed_run:
	go run . build seed.bramble

simple_run:
	go run . build simple/simple.bramble

seed/linux-x86_64-seed.tar.gz:
	./seed/build.sh

integration_tests:
	go test github.com/maxmcd/bramble/pkg/bramble -v -run=Integration

test:
	go test -v ./...

reptar:
	cd pkg/reptar && docker build -t reptar . \
	&& docker run -it reptar sh
