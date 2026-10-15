package grpcclient

import (
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"github.com/azusayn/azushop/internal/pkg/middleware"
	"go.opentelemetry.io/otel/propagation"
)

func NewHTTPClient() *http.Client {
	return &http.Client{}
}

func NewAuthenticatedClientOptions(propagator propagation.TextMapPropagator) ([]connect.ClientOption, error) {
	otelInterceptor, err := otelconnect.NewInterceptor(
		otelconnect.WithPropagator(propagator),
	)
	if err != nil {
		return nil, err
	}
	return []connect.ClientOption{
		connect.WithInterceptors(
			otelInterceptor,
			middleware.AuthClientInterceptor(),
		),
		connect.WithGRPC(),
	}, nil
}

func NewServiceClient[T any](
	newFn func(connect.HTTPClient, string, ...connect.ClientOption) T,
	addr string,
	opts []connect.ClientOption,
) T {
	return newFn(NewHTTPClient(), "http://"+addr, opts...)
}
