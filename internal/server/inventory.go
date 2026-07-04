package server

import (
	inventorypb "azushop/api/inventory/v1"
	"azushop/internal/conf"
	"azushop/internal/pkg/middleware"
	"azushop/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func NewInventoryGRPCServer(c *conf.Server,
	inventoryService *service.InventoryService,
	tracerProvider *sdktrace.TracerProvider,
	logger log.Logger) *grpc.Server {
	var opts = []grpc.ServerOption{
		grpc.Middleware(
			tracing.Server(tracing.WithTracerProvider(tracerProvider)),
			middleware.MetricsInterceptor(),
			recovery.Recovery(),
		),
	}
	if c.Grpc.Network != "" {
		opts = append(opts, grpc.Network(c.Grpc.Network))
	}
	if c.Grpc.Addr != "" {
		opts = append(opts, grpc.Address(c.Grpc.Addr))
	}
	if c.Grpc.Timeout != nil {
		opts = append(opts, grpc.Timeout(c.Grpc.Timeout.AsDuration()))
	}
	srv := grpc.NewServer(opts...)
	inventorypb.RegisterInventoryServiceServer(srv, inventoryService)
	return srv
}

func NewInventoryHTTPServer(c *conf.Server,
	inventoryService *service.InventoryService,
	logger log.Logger) *http.Server {
	var opts = []http.ServerOption{
		http.Filter(middleware.CORSFilter(nil)),
		http.Middleware(
			recovery.Recovery(),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, http.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, http.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, http.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := http.NewServer(opts...)
	inventorypb.RegisterInventoryServiceHTTPServer(srv, inventoryService)
	return srv
}
