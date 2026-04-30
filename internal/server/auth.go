package server

import (
	auth "azushop/api/auth/v1"
	"azushop/internal/conf"
	"azushop/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"
)

func NewAuthGRPCServer(
	serverConf *conf.Server,
	authService *service.AuthService,
	logger log.Logger) *grpc.Server {
	var opts = []grpc.ServerOption{
		grpc.Middleware(
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
	auth.RegisterAuthServiceServer(srv, authService)
	return srv
}

func NewAuthHTTPServer(c *conf.Server,
	authService *service.AuthService,
	logger log.Logger) *http.Server {
	var opts = []http.ServerOption{
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
	auth.RegisterAuthServiceHTTPServer(srv, authService)
	return srv
}
