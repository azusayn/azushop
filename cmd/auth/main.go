package main

import (
	"flag"

	"connectrpc.com/connect"
	cfg "github.com/azusayn/azushop/internal/pkg/config"
	"github.com/azusayn/azushop/internal/pkg/middleware"
	"github.com/azusayn/azushop/internal/server"
	"github.com/azusayn/azushop/internal/service"
	authv1connect "github.com/azusayn/azushop/proto/api/auth/v1/v1connect"
	"github.com/azusayn/azushop/proto/conf"
)

func newConnectServerConfig(connectHandler *service.AuthServiceConnectHandler, config *conf.Server) *server.ConnectServerConfig {
	path, handler := authv1connect.NewAuthServiceHandler(
		connectHandler,
		connect.WithInterceptors(
			middleware.MetricsInterceptor(),
		),
	)
	return &server.ConnectServerConfig{
		Handler: handler,
		Address: config.GetConnectServerAddr(),
		Path:    path,
	}
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
	flag.StringVar(&configPath, "conf", "configs/auth.yaml", "config path, e.g. -conf config.yaml")
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
