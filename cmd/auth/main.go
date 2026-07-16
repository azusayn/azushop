package main

import (
	"context"
	"flag"
	"net/http"
	"os/signal"
	"syscall"

	cfg "github.com/azusayn/azushop/internal/pkg/config"
	"github.com/azusayn/azushop/internal/pkg/log"
	"github.com/azusayn/azushop/internal/pkg/middleware"
	"github.com/azusayn/azushop/internal/runner"
	"github.com/azusayn/azushop/internal/server"
	"github.com/azusayn/azushop/internal/service"
	"github.com/azusayn/azushop/proto/conf"
	authv1connect "github.com/azusayn/azushop/proto/api/auth/v1/v1connect"

	"golang.org/x/sync/errgroup"

	"connectrpc.com/connect"
)

var flagconf string

// App holds the components for the auth service.
type App struct {
	Server        *http.Server
	MetricsRunner *runner.MetricsRunner
}

// newApp creates the App from wire-injected dependencies.
func newApp(
	serverConfig *conf.Server,
	config *conf.Data,
	connectHandler *service.AuthServiceConnectHandler,
	metricsRunner *runner.MetricsRunner,
) (*App, error) {
	path, handler := authv1connect.NewAuthServiceHandler(
		connectHandler,
		connect.WithInterceptors(
			middleware.MetricsInterceptor(),
		),
	)

	mux := http.NewServeMux()
	mux.Handle(path, handler)

	return &App{
		Server: &http.Server{
			Addr:    serverConfig.Http.Addr,
			Handler: middleware.CORSFilter(nil)(mux),
		},
		MetricsRunner: metricsRunner,
	}, nil
}

func init() {
	flag.StringVar(&flagconf, "conf", "configs/auth.yaml", "config path, eg: -conf config.yaml")
}

func main() {
	flag.Parse()
	log.SetupLogger("azushop.auth")

	var bc conf.Bootstrap
	if err := cfg.LoadYAMLConfig(flagconf, &bc); err != nil {
		panic(err)
	}

	tp, tpCleanup, err := server.NewGlobalTraceProvider(bc.Data)
	if err != nil {
		panic(err)
	}
	defer tpCleanup()

	app, cleanup, err := wireApp(bc.Server, bc.Data)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	g, egCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return app.Server.ListenAndServe()
	})
	g.Go(func() error {
		return app.MetricsRunner.Start(egCtx)
	})
	g.Go(func() error {
		<-egCtx.Done()
		tp.Shutdown(context.Background())
		app.MetricsRunner.Stop(context.Background())
		return app.Server.Shutdown(context.Background())
	})
	if err := g.Wait(); err != nil {
		panic(err)
	}
}
