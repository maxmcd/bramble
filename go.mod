module github.com/maxmcd/bramble

go 1.15

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/certifi/gocertifi v0.0.0-20200922220541-2c3bb06c6054
	github.com/containerd/console v1.0.0
	github.com/davecgh/go-spew v1.1.1
	github.com/fsouza/go-dockerclient v1.6.5
	github.com/golang/protobuf v1.4.2
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
	google.golang.org/protobuf v1.23.0
	gopkg.in/hlandau/svcutils.v1 v1.0.10
)

replace (
	github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305
	github.com/mholt/archiver/v3 => github.com/maxmcd/archiver/v3 v3.3.2-0.20200926140316-5fd9d38b8b8b
	go.starlark.net => github.com/maxmcd/starlark-go v0.0.0-20201021154825-b2f805d0d122
)
