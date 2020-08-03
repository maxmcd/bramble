

bramblescript_shell:
	go run . script

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

bramblescripts_to_test:
	go install
	pkg/bramblescript/run.bramble
	pkg/bramblescript/examples.bramble
