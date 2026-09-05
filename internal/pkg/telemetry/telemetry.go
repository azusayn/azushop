package telemetry

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func NewTextMapPropagator() propagation.TextMapPropagator {
	p := propagation.TraceContext{}
	otel.SetTextMapPropagator(p)
	return p
}

func ExtractTraceHeaderBytes(ctx context.Context) ([]byte, error) {
	carrier := make(propagation.MapCarrier)
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	bytes, err := json.Marshal(carrier)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func InjectTraceHeaderBytes(ctx context.Context, headerBytes []byte) (context.Context, error) {
	var carrier propagation.MapCarrier
	if err := json.Unmarshal(headerBytes, &carrier); err != nil {
		return context.Background(), err
	}
	return otel.GetTextMapPropagator().Extract(ctx, carrier), nil
}

func InjectTraceMap(ctx context.Context) map[string]string {
	carrier := make(propagation.MapCarrier)
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier
}

func ContextWithTraceMap(ctx context.Context, m map[string]string) context.Context {
	if len(m) == 0 {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(m))
}

func WithUnsampledSpanContext(ctx context.Context) context.Context {
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		// A valid span (both TraceID and SpanID are not zero) with TraceFlags set to 0x0
		// forces the span to be unsampled. We supply dummy IDs here to satisfy this requirement.
		// Ref: https://github.com/open-telemetry/opentelemetry-go/blob/b62d92831b2dd142f5a0cc89c828270274196877/sdk/trace/sampling.go#L281
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{1},
		TraceFlags: trace.TraceFlags(0),
	})
	return trace.ContextWithSpanContext(ctx, sc)
}
