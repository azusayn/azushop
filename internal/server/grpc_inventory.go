package server

import (
	inventorypb "azushop/api/inventory/v1"
	"azushop/internal/conf"
	"azushop/internal/data"
	"azushop/internal/pkg/middleware"
	"azushop/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
)

func NewInventoryGRPCServer(c *conf.Server,
	inventoryService *service.InventoryService,
	config *data.Data,
	logger log.Logger) *grpc.Server {
	var opts = []grpc.ServerOption{
		grpc.Middleware(
			recovery.Recovery(),
			middleware.AuthInterceptor(&config.GetPrivateKey().PublicKey, config.GetAppName()),
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
