module github.com/maxmcd/bramble

go 1.16

require (
	github.com/BurntSushi/toml v0.4.1
	github.com/bmatcuk/doublestar/v4 v4.0.2
	github.com/certifi/gocertifi v0.0.0-20210507211836-431795d63e8d
	github.com/charmbracelet/bubbletea v0.15.0
	github.com/charmbracelet/lipgloss v0.4.0
	github.com/containerd/console v1.0.3
	github.com/djherbis/buffer v1.2.0
	github.com/julienschmidt/httprouter v1.3.0
	github.com/maxmcd/dag v0.0.0-20210909010249-5757e2034a95
	github.com/mholt/archiver/v3 v3.5.0
	github.com/minio/sha256-simd v1.0.0
	github.com/mitchellh/go-wordwrap v1.0.1
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6
	github.com/opencontainers/runc v1.0.2
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.7.0
	github.com/urfave/cli/v2 v2.3.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.25.0
	go.opentelemetry.io/otel v1.0.1
	go.opentelemetry.io/otel/exporters/jaeger v1.0.1
	go.opentelemetry.io/otel/sdk v1.0.1
	go.opentelemetry.io/otel/trace v1.0.1
	go.starlark.net v0.0.0-20210901212718-87f333178d59
	go.uber.org/zap v1.19.1
	golang.org/x/sys v0.0.0-20211007075335-d3039528d8ac
)

replace (
	github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305
	github.com/mholt/archiver/v3 => github.com/maxmcd/archiver/v3 v3.3.2-0.20210923004632-06ef4f8f175b
	go.starlark.net => github.com/maxmcd/starlark-go v0.0.0-20201021154825-b2f805d0d122
)
