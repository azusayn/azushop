package runner

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsRunner serves a Prometheus metrics endpoint.
type MetricsRunner struct {
	httpServer *http.Server
}

func NewMetricsRunner() *MetricsRunner {
	return &MetricsRunner{}
}

func (r *MetricsRunner) Start(_ context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	// TODO: make port configurable.
	r.httpServer = &http.Server{
		Addr:    ":9090",
		Handler: mux,
	}
	if err := r.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (r *MetricsRunner) Stop(ctx context.Context) error {
	return r.httpServer.Shutdown(ctx)
}
