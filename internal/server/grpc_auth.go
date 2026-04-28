package server

import (
	auth "azushop/api/auth/v1"
	"azushop/internal/conf"
	"azushop/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
)

func NewAuthGRPCServer(
	serverConf *conf.Server,
	authService *service.AuthServiceService,
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
