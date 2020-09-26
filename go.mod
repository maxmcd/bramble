module github.com/maxmcd/bramble

go 1.14

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/containerd/console v1.0.0
	github.com/davecgh/go-spew v1.1.1
	github.com/hashicorp/terraform v0.13.2
	github.com/jaguilar/vt100 v0.0.0-20150826170717-2703a27b14ea
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/mholt/archiver/v3 v3.3.1-0.20200626164424-d44471c49aa7
	github.com/mitchellh/cli v1.1.1
	github.com/moby/moby v1.13.1
	github.com/morikuni/aec v1.0.0
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.6.1
	github.com/tonistiigi/units v0.0.0-20180711220420-6950e57a87ea
	go.starlark.net v0.0.0-20200901195727-6e684ef5eeee
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e
)

replace (
	github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305
	github.com/mholt/archiver/v3 => github.com/maxmcd/archiver/v3 v3.3.2-0.20200926140316-5fd9d38b8b8b
)
