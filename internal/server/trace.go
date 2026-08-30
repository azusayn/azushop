package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
)

const (
	TracerProviderInitTimeout    time.Duration = time.Second * 3
	TracerProviderCleanupTimeout time.Duration = time.Second * 2
)

func NewGlobalTraceProvider(serviceName string, gRPCAddress string) (*sdktrace.TracerProvider, func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), TracerProviderInitTimeout)
	defer cancel()

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(gRPCAddress),
	)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create tracer exporter: %v", err)
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), TracerProviderCleanupTimeout)
		slog.Info(fmt.Sprintf("global tracer provider will shutdown in %.1f second(s)", TracerProviderCleanupTimeout.Seconds()))
		defer cancel()
		_ = tp.ForceFlush(ctx)
		_ = tp.Shutdown(ctx)
	}

	return tp, cleanup, nil
}
