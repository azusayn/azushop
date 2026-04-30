package server

import (
	paymentpb "azushop/api/payment/v1"
	"azushop/internal/conf"
	"azushop/internal/pkg/middleware"
	"azushop/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"
)

func NewPaymentGRPCServer(c *conf.Server,
	paymentService *service.PaymentService,
	logger log.Logger) *grpc.Server {
	var opts = []grpc.ServerOption{
		grpc.Middleware(
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
	paymentpb.RegisterPaymentServiceServer(srv, paymentService)
	return srv
}

func NewPaymentHTTPServer(
	config *conf.Server,
	paymentService *service.PaymentService,
) *http.Server {
	opts := []http.ServerOption{http.Middleware(recovery.Recovery())}
	if config.Http.Network != "" {
		opts = append(opts, http.Network(config.Http.Network))
	}
	if config.Http.Addr != "" {
		opts = append(opts, http.Address(config.Http.Addr))
	}
	if config.Http.Timeout != nil {
		opts = append(opts, http.Timeout(config.Http.Timeout.AsDuration()))
	}
	srv := http.NewServer(opts...)
	paymentpb.RegisterPaymentServiceHTTPServer(srv, paymentService)
	return nil
}
