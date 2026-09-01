package main

import (
	"flag"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	cfg "github.com/azusayn/azushop/internal/pkg/config"
	"github.com/azusayn/azushop/internal/pkg/crypto"
	"github.com/azusayn/azushop/internal/pkg/middleware"
	"github.com/azusayn/azushop/internal/server"
	"github.com/azusayn/azushop/internal/service"
	productv1connect "github.com/azusayn/azushop/proto/api/product/v1/v1connect"
	"github.com/azusayn/azushop/proto/conf"
)

func newConnectServerConfig(connectHandler *service.ProductServiceConnectHandler, dataConfig *conf.Data, config *conf.Server) (*server.ConnectServerConfig, error) {
	publicKey, err := crypto.LoadEd25519PublicKey(dataConfig.GetAuth().GetPublicKeyPath())
	if err != nil {
		return nil, err
	}

	connectInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		return nil, err
	}
	path, handler := productv1connect.NewProductServiceHandler(
		connectHandler,
		connect.WithInterceptors(
			connectInterceptor,
			middleware.AuthInterceptor(publicKey, dataConfig.GetAuth().GetIssuer(), false),
			middleware.MetricsInterceptor(),
		),
	)
	return &server.ConnectServerConfig{
		ServiceName: productv1connect.ProductServiceName,
		Handlers: map[string]http.Handler{
			path: handler,
		},
		Address: config.GetConnectServerAddr(),
	}, nil
}

type App struct {
	server.Runtime
}

func newApp(connectServer *server.ConnectServer, metricsServer *server.MetricsServer) *App {
	app := &App{}
	app.Servers = []server.Server{connectServer, metricsServer}
	return app
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "conf", "configs/product.yaml", "config path, e.g. -conf config.yaml")
	config, err := cfg.LoadConfig(configPath)
	if err != nil {
		panic(err)
	}

	app, cleanup, err := wireApp(config.Data, config.Server)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	app.Bootstrap(config.Data)
}
