//go:build wireinject
// +build wireinject

package main

import (
	"github.com/azusayn/azushop/internal/biz"
	"github.com/azusayn/azushop/internal/data"
	"github.com/azusayn/azushop/internal/runner"
	"github.com/azusayn/azushop/internal/server"
	"github.com/azusayn/azushop/internal/service"
	"github.com/azusayn/azushop/proto/conf"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

func wireInventoryApp(*conf.Server, *conf.Data, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(
		server.NewInventoryGRPCServer,
		server.NewInventoryHTTPServer,
		data.InventoryDataProviderSet,
		biz.NewInventoryUsecase,
		service.NewInventoryService,
		runner.NewInventoryRunner,
		server.NewGlobalTraceProvider,
		newApp,
	))
}
