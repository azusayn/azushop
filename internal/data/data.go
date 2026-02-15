package data

import (
	"azushop/internal/conf"
	"crypto/rsa"
	"database/sql"
	"log/slog"

	"github.com/azusayn/azutils/auth"
	"github.com/google/wire"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(
	NewData,
	NewGreeterRepo,
	NewUserRepo,
)

type Data struct {
	// TODO: wrapped redis client
	// TODO: DDD design.
	postgresClient *sql.DB
	PrivateKey     *rsa.PrivateKey
	AppName        string
}

func NewData(c *conf.Data) (*Data, func(), error) {
	key, err := auth.GeneratePrivateKey()
	if err != nil {
		return nil, nil, err
	}

	postgresClient, err := sql.Open(c.Database.Driver, c.Database.Source)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		slog.Warn("close postgres connection...")
		err := postgresClient.Close()
		if err != nil {
			slog.Warn(err.Error())
		}
	}

	return &Data{
		PrivateKey:     key,
		postgresClient: postgresClient,
		AppName:        c.AppName,
	}, cleanup, nil
}
