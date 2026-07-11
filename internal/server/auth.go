package server

import (
	"github.com/azusayn/azushop/internal/pkg/middleware"
	"github.com/azusayn/azushop/internal/service"
	authpb "github.com/azusayn/azushop/proto/api/auth/v1"
	authv1connect "github.com/azusayn/azushop/proto/api/auth/v1/v1connect"
	"github.com/azusayn/azushop/proto/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func NewAuthGRPCServer(
	serverConf *conf.Server,
	authService *service.AuthService,
	tracerProvider *sdktrace.TracerProvider,
	logger log.Logger,
) *grpc.Server {
	var opts = []grpc.ServerOption{
		grpc.Middleware(
			tracing.Server(tracing.WithTracerProvider(tracerProvider)),
			middleware.MetricsInterceptor(),
			recovery.Recovery(),
			logging.Server(logger),
		),
	}
	if serverConf.Grpc.Network != "" {
		opts = append(opts, grpc.Network(serverConf.Grpc.Network))
	}
	if serverConf.Grpc.Addr != "" {
		opts = append(opts, grpc.Address(serverConf.Grpc.Addr))
	}
	if serverConf.Grpc.Timeout != nil {
		opts = append(opts, grpc.Timeout(serverConf.Grpc.Timeout.AsDuration()))
	}
	srv := grpc.NewServer(opts...)
	authpb.RegisterAuthServiceServer(srv, authService)
	return srv
}

func NewAuthHTTPServer(c *conf.Server,
	authService *service.AuthService,
	logger log.Logger) *kratoshttp.Server {
	opts := []kratoshttp.ServerOption{
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
	connectHandler := service.NewAuthServiceConnectHandler(authService)
	path, handler := authv1connect.NewAuthServiceHandler(connectHandler)
	srv.HandlePrefix(path, handler)
	return srv
}
