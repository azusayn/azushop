package server

import (
	auth "azushop/api/auth/v1"
	inventorypb "azushop/api/inventory/v1"
	orderpb "azushop/api/order/v1"
	paymentpb "azushop/api/payment/v1"
	productpb "azushop/api/product/v1"
	"azushop/internal/conf"
	"azushop/internal/data"
	"azushop/internal/pkg/middleware"
	"azushop/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
)

// NewGRPCServer new a gRPC server.
func NewGRPCServer(c *conf.Server,
	authService *service.AuthServiceService,
	productService *service.ProductService,
	inventoryService *service.InventoryService,
	orderService *service.OrderService,
	paymentService *service.PaymentService,
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
	auth.RegisterAuthServiceServer(srv, authService)
	productpb.RegisterProductServiceServer(srv, productService)
	inventorypb.RegisterInventoryServiceServer(srv, inventoryService)
	orderpb.RegisterOrderServiceServer(srv, orderService)
	paymentpb.RegisterPaymentServiceServer(srv, paymentService)
	return srv
}
