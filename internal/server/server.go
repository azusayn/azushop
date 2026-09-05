package server

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/azusayn/azushop/internal/pkg/log"
	"github.com/azusayn/azushop/proto/conf"
	"go.opentelemetry.io/otel"
)

const (
	ServerShutdownTimeout time.Duration = 5 * time.Second
)

type Server interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type Runtime struct {
	Servers []Server
}

func NewRuntime() *Runtime {
	return &Runtime{}
}

func (r *Runtime) Bootstrap(config *conf.Data) {
	appName := config.GetAppName()
	log.SetupGlobalLogger(appName)

	tp, tpCleanup, err := NewGlobalTraceProvider(
		appName,
		config.GetOtlpGrpcAddress(),
	)
	if err != nil {
		slog.Warn("failed to get tracer provider", slog.Any("error", err))
	} else {
		otel.SetTracerProvider(tp)
		defer tpCleanup()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-quit
		slog.Info("signal received", slog.Any("signal", sig))

		if err := r.stopAll(); err != nil {
			slog.Error("failed to stop server", slog.Any("error", err))
		}
	}()

	if err := r.startAll(context.TODO()); err != nil {
		slog.Error("server exited with error", slog.Any("error", err))
	}

	slog.Info("server stopped")
}

func (r *Runtime) startAll(ctx context.Context) error {
	n := len(r.Servers)
	ch := make(chan error, n)

	var wg sync.WaitGroup
	for _, server := range r.Servers {
		wg.Go(func() {
			ch <- server.Start(ctx)
		})
	}
	wg.Wait()

	return collectNErrors(n, ch)
}

func (r *Runtime) stopAll() error {
	ctx, cancel := context.WithTimeout(context.Background(), ServerShutdownTimeout)
	defer cancel()

	n := len(r.Servers)
	ch := make(chan error, n)

	var wg sync.WaitGroup

	for _, server := range r.Servers {
		wg.Go(func() {
			ch <- server.Stop(ctx)
		})
	}

	wg.Wait()

	return collectNErrors(n, ch)
}

func collectNErrors(n int, ch chan error) error {
	var err error
	for i := 0; i < n; i++ {
		err = errors.Join(err, <-ch)
	}
	return err
}
