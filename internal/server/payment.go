package server

import (
	"github.com/azusayn/azushop/internal/pkg/crypto"
	"github.com/azusayn/azushop/internal/pkg/middleware"
	"github.com/azusayn/azushop/internal/service"
	"github.com/azusayn/azushop/proto/conf"

	paymentpb "github.com/azusayn/azushop/proto/api/payment/v1"
	paymentv1connect "github.com/azusayn/azushop/proto/api/payment/v1/v1connect"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func NewPaymentGRPCServer(
	cs *conf.Server,
	cd *conf.Data,
	paymentService *service.PaymentService,
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
) *kratoshttp.Server {
	opts := []kratoshttp.ServerOption{
		kratoshttp.Filter(middleware.CORSFilter(nil)),
		kratoshttp.Middleware(recovery.Recovery()),
	}
	if config.Http.Network != "" {
		opts = append(opts, kratoshttp.Network(config.Http.Network))
	}
	if config.Http.Addr != "" {
		opts = append(opts, kratoshttp.Address(config.Http.Addr))
	}
	if config.Http.Timeout != nil {
		opts = append(opts, kratoshttp.Timeout(config.Http.Timeout.AsDuration()))
	}
	srv := kratoshttp.NewServer(opts...)
	connectHandler := service.NewPaymentServiceConnectHandler(paymentService)
	path, handler := paymentv1connect.NewPaymentServiceHandler(connectHandler)
	srv.HandlePrefix(path, handler)
	return srv
}
