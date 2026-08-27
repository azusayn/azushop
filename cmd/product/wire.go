//go:build wireinject
// +build wireinject

package main

import (
	"github.com/azusayn/azushop/internal/biz"
	"github.com/azusayn/azushop/internal/data"
	"github.com/azusayn/azushop/internal/pkg/llm"
	"github.com/azusayn/azushop/internal/server"
	"github.com/azusayn/azushop/internal/service"
	"github.com/azusayn/azushop/proto/conf"
	"github.com/google/wire"
)

func wireApp(cd *conf.Data, cs *conf.Server) (*App, func(), error) {
	panic(wire.Build(wireProviders))
}

var wireProviders = wire.NewSet(
	data.ProductDataProviderSet,
	biz.NewProductUsecase,
	llm.NewOpenAIClient,
	service.NewProductService,
	service.NewProductServiceConnectHandler,
	newConnectServerConfig,
	server.NewConnectServer,
	server.NewMetricsServer,
	newApp,
)
