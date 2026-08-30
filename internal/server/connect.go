package server

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/grpcreflect"
	"github.com/azusayn/azushop/internal/pkg/middleware"
)

type ConnectServer struct {
	HTTPServer *http.Server
	BasePath   string
}

type ConnectServerConfig struct {
	// ServiceName is used for gRPC reflection.
	ServiceName string
	Address     string
	// Handlers is a map from serveMux pattern to http.Handler.
	Handlers map[string]http.Handler
}

func NewConnectServer(config *ConnectServerConfig) *ConnectServer {
	mux := http.NewServeMux()

	if config.ServiceName != "" {
		reflector := grpcreflect.NewStaticReflector(config.ServiceName)
		mux.Handle(grpcreflect.NewHandlerV1(reflector))
	}

	var entryPointHandler http.Handler
	for pattern, handler := range config.Handlers {
		mux.Handle(pattern, handler)
	}
	entryPointHandler = middleware.CORSFilter(nil)(mux)

	// enable HTTP/1.x and unencrypted HTTP/2.
	// Ref: https://pkg.go.dev/golang.org/x/net/http2/h2c#pkg-overview
	protocol := new(http.Protocols)
	protocol.SetHTTP1(true)
	protocol.SetUnencryptedHTTP2(true)
	return &ConnectServer{
		HTTPServer: &http.Server{
			Addr:      config.Address,
			Handler:   entryPointHandler,
			Protocols: protocol,
		},
	}
}

func (c *ConnectServer) Start(ctx context.Context) error {
	slog.Info(
		"Connect server is listening",
		slog.Any("addr", c.HTTPServer.Addr),
		slog.Any("protocols", "http/1.x, grpc (http/2)"),
	)
	if err := c.HTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (c *ConnectServer) Stop(ctx context.Context) error {
	return c.HTTPServer.Close()
}

var _ Server = (*ConnectServer)(nil)
