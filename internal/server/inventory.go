package server

import (
	"github.com/azusayn/azushop/internal/pkg/middleware"
	"github.com/azusayn/azushop/internal/service"
	inventorypb "github.com/azusayn/azushop/proto/api/inventory/v1"
	inventoryv1connect "github.com/azusayn/azushop/proto/api/inventory/v1/v1connect"
	"github.com/azusayn/azushop/proto/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
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
	logger log.Logger) *kratoshttp.Server {
	var opts = []kratoshttp.ServerOption{
		kratoshttp.Filter(middleware.CORSFilter(nil)),
		kratoshttp.Middleware(
			recovery.Recovery(),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, kratoshttp.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, kratoshttp.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, kratoshttp.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := kratoshttp.NewServer(opts...)
	connectHandler := service.NewInventoryServiceConnectHandler(inventoryService)
	path, handler := inventoryv1connect.NewInventoryServiceHandler(connectHandler)
	srv.HandlePrefix(path, handler)
	return srv
}
