//go:build wireinject
// +build wireinject

package main

import (
	"github.com/azusayn/azushop/internal/biz"
	"github.com/azusayn/azushop/internal/data"
	"github.com/azusayn/azushop/internal/runner"
	"github.com/azusayn/azushop/internal/service"
	"github.com/azusayn/azushop/proto/conf"
	"github.com/google/wire"
)

func wireApp(serverConfig *conf.Server, dataConfig *conf.Data) (*App, func(), error) {
	panic(wire.Build(wireProviders))
}

var wireProviders = wire.NewSet(
	data.OrderDataProviderSet,
	biz.NewOrderUsecase,
	service.NewOrderService,
	service.NewOrderServiceConnectHandler,
	runner.NewOrderRunner,
	newApp,
)
