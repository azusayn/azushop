package server

import (
	"context"
	"net/http"

	"github.com/azusayn/azushop/proto/conf"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	MetricsServerPath string = "/metrics"
)

type MetricsServer struct {
	httpServer *http.Server
}

func NewMetricsServer(config *conf.Server) *MetricsServer {
	m := &MetricsServer{}
	mux := http.NewServeMux()
	mux.Handle(MetricsServerPath, promhttp.Handler())
	m.httpServer = &http.Server{
		Addr:    config.GetMetricsServerAddr(),
		Handler: mux,
	}
	return m
}

func (m *MetricsServer) Start(ctx context.Context) error {
	if err := m.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (m *MetricsServer) Stop(ctx context.Context) error {
	return m.httpServer.Shutdown(ctx)
}

var _ Server = (*MetricsServer)(nil)
