//go:build wireinject
// +build wireinject

package main

import (
	"azushop/internal/biz"
	"azushop/internal/conf"
	"azushop/internal/data"
	"azushop/internal/runner"
	"azushop/internal/server"
	"azushop/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

func wireInventoryApp(*conf.Server, *conf.Data, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(
		server.NewInventoryGRPCServer,
		data.InventoryDataProviderSet,
		biz.NewInventoryUsecase,
		service.NewInventoryService,
		runner.NewInventoryRunner,
		newApp,
	))
}
