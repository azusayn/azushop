//go:build wireinject
// +build wireinject

package main

import (
	"azushop/internal/biz"
	"azushop/internal/conf"
	"azushop/internal/data"
	"azushop/internal/server"
	"azushop/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

func wirePaymentApp(*conf.Server, *conf.Data, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(
		server.NewPaymentGRPCServer,
		data.NewPaymentPublisher,
		data.ProviderSet,
		biz.NewPaymentUsecase,
		service.NewPaymentService,
		newApp,
	))
}
