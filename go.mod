module github.com/maxmcd/bramble

go 1.16

require (
	github.com/BurntSushi/toml v0.4.1
	github.com/bmatcuk/doublestar/v4 v4.0.2
	github.com/certifi/gocertifi v0.0.0-20210507211836-431795d63e8d
	github.com/containerd/console v1.0.3
	github.com/creack/pty v1.1.15 // indirect
	github.com/jaguilar/vt100 v0.0.0-20201024211400-81de19cb81a4
	github.com/maxmcd/dag v0.0.0-20210909010249-5757e2034a95
	github.com/mholt/archiver/v3 v3.5.0
	github.com/minio/sha256-simd v1.0.0
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6
	github.com/morikuni/aec v1.0.0
	github.com/opencontainers/runc v1.0.2
	github.com/peterbourgon/ff/v3 v3.1.0
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.7.0
	github.com/tonistiigi/units v0.0.0-20180711220420-6950e57a87ea
	go.starlark.net v0.0.0-20210901212718-87f333178d59
	go.uber.org/zap v1.19.1
	golang.org/x/sys v0.0.0-20210909193231-528a39cd75f3
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

replace (
	github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305
	github.com/mholt/archiver/v3 => github.com/maxmcd/archiver/v3 v3.3.2-0.20210910013223-bd3a04eff5a3
	go.starlark.net => github.com/maxmcd/starlark-go v0.0.0-20201021154825-b2f805d0d122
)
