module github.com/maxmcd/bramble

go 1.16

require (
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/BurntSushi/toml v0.3.1
	github.com/bmatcuk/doublestar/v4 v4.0.1
	github.com/certifi/gocertifi v0.0.0-20200922220541-2c3bb06c6054
	github.com/containerd/console v1.0.0
	github.com/creack/pty v1.1.11
	github.com/docker/docker v1.4.2-0.20191101170500-ac7306503d23
	github.com/go-git/go-git/v5 v5.3.0
	github.com/google/go-cmp v0.5.5 // indirect
	github.com/jaguilar/vt100 v0.0.0-20201024211400-81de19cb81a4
	github.com/maxmcd/dag v0.0.0-20210316172417-f02e4b03c6e9
	github.com/mholt/archiver/v3 v3.3.1-0.20200626164424-d44471c49aa7
	github.com/minio/sha256-simd v1.0.0
	github.com/morikuni/aec v1.0.0
	github.com/peterbourgon/ff/v3 v3.0.0
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.6.1
	github.com/tonistiigi/units v0.0.0-20180711220420-6950e57a87ea
	go.starlark.net v0.0.0-20200901195727-6e684ef5eeee
	go.uber.org/zap v1.10.0
	golang.org/x/sys v0.0.0-20210324051608-47abb6519492
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e
	gotest.tools v2.2.0+incompatible // indirect
)

replace (
	github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305
	github.com/mholt/archiver/v3 => github.com/maxmcd/archiver/v3 v3.3.2-0.20200926140316-5fd9d38b8b8b
	go.starlark.net => github.com/maxmcd/starlark-go v0.0.0-20201021154825-b2f805d0d122
)
