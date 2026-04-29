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

func wireAuthApp(*conf.Server, *conf.Data, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(
		server.NewAuthGRPCServer,
		server.NewAuthHTTPServer,
		data.AuthDataProviderSet,
		biz.NewUserUsecase,
		service.NewAuthService,
		newApp,
	))
}
