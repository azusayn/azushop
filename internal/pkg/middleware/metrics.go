package middleware

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc/status"

	"connectrpc.com/connect"
)

var (
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "grpc_request_duration_seconds",
			Help:    "gRPC response duration secondes",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5},
		},
		[]string{"method", "status_code"},
	)
	RequestTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grpc_request_total",
			Help: "total gRPC requests",
		},
		[]string{"method", "status_code"},
	)
	RequestInFlights = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "grpc_request_in_flight",
		Help: "gRPC requests that are in flight",
	},
		[]string{"method"},
	)
)

func MetricsInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			start := time.Now()
			procedure := req.Spec().Procedure
			RequestInFlights.WithLabelValues(procedure).Inc()
			defer RequestInFlights.WithLabelValues(procedure).Dec()

			resp, err := next(ctx, req)
			code := status.Code(err).String()
			elapsed := time.Since(start).Seconds()

			RequestDuration.WithLabelValues(procedure, code).Observe(elapsed)
			RequestTotal.WithLabelValues(procedure, code).Inc()

			return resp, err
		}
	}
}
