package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/azusayn/azushop/internal/pkg/middleware"
)

type ConnectServer struct {
	HTTPServer *http.Server
	Path       string
}

type ConnectServerConfig struct {
	Handler http.Handler
	Address string
	Path    string
}

func NewConnectServer(config *ConnectServerConfig) *ConnectServer {
	mux := http.NewServeMux()
	mux.Handle(config.Path, config.Handler)
	config.Handler = middleware.CORSFilter(nil)(mux)
	return &ConnectServer{
		HTTPServer: &http.Server{
			Addr:    config.Address,
			Handler: config.Handler,
		},
		Path: config.Path,
	}
}

func (c *ConnectServer) Start(ctx context.Context) error {
	slog.Info("connect server is listening", slog.Any("addr", c.HTTPServer.Addr))
	if err := c.HTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (c *ConnectServer) Stop(ctx context.Context) error {
	return c.HTTPServer.Close()
}

var _ Server = (*ConnectServer)(nil)
