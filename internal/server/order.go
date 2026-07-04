package server

import (
	orderpb "azushop/api/order/v1"
	"azushop/internal/conf"
	"azushop/internal/pkg/crypto"
	"azushop/internal/pkg/middleware"
	"azushop/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func NewOrderGRPCServer(
	cs *conf.Server,
	cd *conf.Data,
	orderService *service.OrderService,
	tracerProvider *sdktrace.TracerProvider,
	logger log.Logger) (*grpc.Server, error) {
	publicKey, err := crypto.LoadEd25519PublicKey(cd.GetAuth().GetPublicKeyPath())
	if err != nil {
		return nil, err
	}
	var opts = []grpc.ServerOption{
		grpc.Middleware(
			tracing.Server(tracing.WithTracerProvider(tracerProvider)),
			recovery.Recovery(),
			middleware.MetricsInterceptor(),
			middleware.AuthInterceptor(publicKey, cd.GetAuth().GetIssuer(), false),
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
	server := grpc.NewServer(opts...)
	orderpb.RegisterOrderServiceServer(server, orderService)
	return server, nil
}

func NewOrderHTTPServer(c *conf.Server,
	orderService *service.OrderService,
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
	orderpb.RegisterOrderServiceHTTPServer(srv, orderService)
	return srv
}
