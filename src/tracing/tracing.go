package tracing

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	service     = "bramble"
	environment = "production"
	id          = 1
)

// tracerProvider returns an OpenTelemetry TracerProvider configured to use
// the Jaeger exporter that will send spans to the provided url. The returned
// TracerProvider will also use a Resource configured with all the information
// about the application.
func tracerProvider(hostAndPort string) (*tracesdk.TracerProvider, error) {
	// Create the Jaeger exporter

	// TODO: better validation, resolve IP+port?
	parts := strings.Split(hostAndPort, ":")
	jaegerBatcher, err := jaeger.New(jaeger.WithAgentEndpoint(
		jaeger.WithAgentHost(parts[0]),
		jaeger.WithAgentPort(parts[1]),
	))
	if err != nil {
		return nil, err
	}
	return tracesdk.NewTracerProvider(
		// Always be sure to batch in production.
		tracesdk.WithBatcher(jaegerBatcher),
		// Record information about this application in an Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(service),
			attribute.String("environment", environment),
			attribute.Int64("ID", id),
		)),
	), nil
}

var (
	tp *tracesdk.TracerProvider
)

func init() {
	hostAndPort, found := os.LookupEnv("JAEGER_TRACE")
	if !found {
		// Never collect traces if we're not gathering them
		tp = tracesdk.NewTracerProvider(tracesdk.WithSampler(tracesdk.NeverSample()))
		return
	}
	var err error
	tp, err = tracerProvider(hostAndPort)
	if err != nil {
		panic(err)
	}

	otel.SetTracerProvider(tp)
}

func Tracer(name string) trace.Tracer {
	if tp == nil {
		panic("tracing provider hasn't been initialized")
	}
	return tp.Tracer(name)
}

func Stop() {
	if tp == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()
	if err := tp.Shutdown(ctx); err != nil {
		log.Println(err)
	}
}
