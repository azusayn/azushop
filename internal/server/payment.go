package server

import (
	paymentpb "azushop/api/payment/v1"
	"azushop/internal/conf"
	"azushop/internal/pkg/crypto"
	"azushop/internal/pkg/middleware"
	"azushop/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"
)

func NewPaymentGRPCServer(
	cs *conf.Server,
	cd *conf.Data,
	paymentService *service.PaymentService,
	logger log.Logger) (*grpc.Server, error) {
	publicKey, err := crypto.LoadEd25519PublicKey(cd.GetAuth().GetPublicKeyPath())
	if err != nil {
		return nil, err
	}
	var opts = []grpc.ServerOption{
		grpc.Middleware(
			recovery.Recovery(),
			middleware.MetricsInterceptor(),
			middleware.AuthInterceptor(publicKey, cd.Auth.GetIssuer(), false),
		),
	}
	if cs.Grpc.Network != "" {
		opts = append(opts, grpc.Network(cs.Grpc.Network))
	}
	if cs.Grpc.Addr != "" {
		opts = append(opts, grpc.Address(cs.Grpc.Addr))
	}
	if cs.Grpc.Timeout != nil {
		opts = append(opts, grpc.Timeout(cs.Grpc.Timeout.AsDuration()))
	}
	srv := grpc.NewServer(opts...)
	paymentpb.RegisterPaymentServiceServer(srv, paymentService)
	return srv, nil
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
	return srv
}
