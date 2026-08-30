package main

import (
	"flag"
	"net/http"

	"connectrpc.com/connect"
	cfg "github.com/azusayn/azushop/internal/pkg/config"
	"github.com/azusayn/azushop/internal/pkg/crypto"
	"github.com/azusayn/azushop/internal/pkg/middleware"
	"github.com/azusayn/azushop/internal/server"
	"github.com/azusayn/azushop/internal/service"
	inventoryv1connect "github.com/azusayn/azushop/proto/api/inventory/v1/v1connect"
	"github.com/azusayn/azushop/proto/conf"
)

func newConnectServerConfig(
	connectHandler *service.InventoryServiceConnectHandler,
	serverConfig *conf.Server,
	dataConfig *conf.Data,
) (*server.ConnectServerConfig, error) {
	publicKey, err := crypto.LoadEd25519PublicKey(dataConfig.GetAuth().GetPublicKeyPath())
	if err != nil {
		return nil, err
	}

	path, handler := inventoryv1connect.NewInventoryServiceHandler(
		connectHandler,
		connect.WithInterceptors(
			middleware.AuthInterceptor(publicKey, dataConfig.GetAuth().GetIssuer(), false),
			middleware.MetricsInterceptor(),
		),
	)
	return &server.ConnectServerConfig{
		ServiceName: inventoryv1connect.InventoryServiceName,
		Handlers: map[string]http.Handler{
			path: handler,
		},
		Address: serverConfig.GetConnectServerAddr(),
	}, nil
}

type App struct {
	server.Runtime
}

func newApp(
	connectServer *server.ConnectServer,
	metricsServer *server.MetricsServer,
	inventoryRunner *server.InventoryRunner,
) *App {
	app := &App{}
	app.Servers = []server.Server{connectServer, metricsServer, inventoryRunner}
	return app
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "conf", "configs/inventory.yaml", "config path, e.g. -conf config.yaml")
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
