package middleware

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func MetricsInterceptor() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			start := time.Now()
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return nil, status.Error(codes.Internal, "failed to extract transporter from ctx")
			}
			method := tr.Operation()
			RequestInFlights.WithLabelValues(method).Inc()
			defer RequestInFlights.WithLabelValues(method).Dec()

			resp, err := handler(ctx, req)
			code := status.Code(err).String()
			elapsed := time.Since(start).Seconds()

			RequestDuration.WithLabelValues(method, code).Observe(elapsed)
			RequestTotal.WithLabelValues(method, code).Inc()

			return resp, err
		}
	}
}
