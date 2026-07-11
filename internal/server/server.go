package server

import (
	"context"
	"time"

	"github.com/azusayn/azushop/proto/conf"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

const defaultTraceProviderInitTimeout time.Duration = time.Second * 3

func NewGlobalTraceProvider(conf *conf.Data) (*sdktrace.TracerProvider, func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTraceProviderInitTimeout)
	defer cancel()

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(conf.GetOtlpGrpcAddress()),
	)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create tracer exporter: %v", err)
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(conf.GetAppName()),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)

	cleanup := func() {
		_ = tp.Shutdown(context.Background())
	}

	return tp, cleanup, nil
}
