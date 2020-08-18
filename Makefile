

bramblescript_shell:
	go run . script

seed_run:
	go run . build seed/seed.bramble

simple_run:
	go run . build tests/simple/simple.bramble

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
	bramble script pkg/bramblescript/examples/run.bramble
	bramble script pkg/bramblescript/examples.bramble
