package runner

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsRunner serves a Prometheus metrics endpoint.
type MetricsRunner struct {
}

func NewMetricsRunner() *MetricsRunner {
	return &MetricsRunner{}
}

func (r *MetricsRunner) Start(_ context.Context) error {
	http.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(":9090", nil)
}

func (r *MetricsRunner) Stop(_ context.Context) error {
	return nil
}
