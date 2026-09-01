package telemetry

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

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
