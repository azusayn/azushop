package main

import (
	"context"
	"flag"
	"net/http"
	"os/signal"
	"syscall"

	cfg "github.com/azusayn/azushop/internal/pkg/config"
	"github.com/azusayn/azushop/internal/pkg/crypto"
	"github.com/azusayn/azushop/internal/pkg/log"
	"github.com/azusayn/azushop/internal/pkg/middleware"
	"github.com/azusayn/azushop/internal/runner"
	"github.com/azusayn/azushop/internal/server"
	"github.com/azusayn/azushop/internal/service"
	"github.com/azusayn/azushop/proto/conf"
	orderv1connect "github.com/azusayn/azushop/proto/api/order/v1/v1connect"

	"golang.org/x/sync/errgroup"

	"connectrpc.com/connect"
)

var flagconf string

// App holds the components for the order service.
type App struct {
	Server      *http.Server
	OrderRunner *runner.OrderRunner
}

// newApp creates the App from wire-injected dependencies.
func newApp(
	serverConfig *conf.Server,
	config *conf.Data,
	connectHandler *service.OrderServiceConnectHandler,
	orderRunner *runner.OrderRunner,
) (*App, error) {
	publicKey, err := crypto.LoadEd25519PublicKey(config.GetAuth().GetPublicKeyPath())
	if err != nil {
		return nil, err
	}

	path, handler := orderv1connect.NewOrderServiceHandler(
		connectHandler,
		connect.WithInterceptors(
			middleware.AuthInterceptor(publicKey, config.GetAuth().GetIssuer(), false),
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
		OrderRunner: orderRunner,
	}, nil
}

func init() {
	flag.StringVar(&flagconf, "conf", "configs/order.yaml", "config path, eg: -conf config.yaml")
}

func main() {
	flag.Parse()
	log.SetupLogger("azushop.order")

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
		return app.OrderRunner.Start(egCtx)
	})
	g.Go(func() error {
		<-egCtx.Done()
		tp.Shutdown(context.Background())
		app.OrderRunner.Stop(context.Background())
		return app.Server.Shutdown(context.Background())
	})
	if err := g.Wait(); err != nil {
		panic(err)
	}
}
