

bramblescript_shell:
	go run . script

seed_run:
	go run . run seed:seed

simple_run:
	go run . run tests/simple/simple:simple

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
	bramble run pkg/bramblecmd/examples/run:total_bytes_in_folder
	bramble run pkg/bramblecmd/examples:main

refactor_check:
	go install
	cd pkg/bramblecmd/examples/other_new_structure && bramble run main:main
	cd pkg/bramblecmd/examples/new_structure && bramble test
