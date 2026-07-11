package server

import (
	"github.com/azusayn/azushop/internal/pkg/crypto"
	"github.com/azusayn/azushop/internal/pkg/middleware"
	"github.com/azusayn/azushop/internal/service"

	productpb "github.com/azusayn/azushop/proto/api/product/v1"
	productv1connect "github.com/azusayn/azushop/proto/api/product/v1/v1connect"
	"github.com/azusayn/azushop/proto/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func NewProductGRPCServer(
	cs *conf.Server,
	cd *conf.Data,
	productService *service.ProductService,
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
	srv := grpc.NewServer(opts...)
	productpb.RegisterProductServiceServer(srv, productService)
	return srv, nil
}

func NewProductHTTPServer(c *conf.Server,
	productService *service.ProductService,
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
	connectHandler := service.NewProductServiceConnectHandler(productService)
	path, handler := productv1connect.NewProductServiceHandler(connectHandler)
	srv.HandlePrefix(path, handler)
	return srv
}
