package data

import (
	"azushop/internal/conf"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pkg/errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Postgres struct {
	Conn       *sql.DB
	GormClient *gorm.DB
}

func NewPostgres(config *conf.Data) (*Postgres, error) {
	if config == nil {
		return nil, errors.New("nil PostgresConfig")
	}
	postgresConn, err := sql.Open(
		config.GetDatabase().GetDriver(),
		config.GetDatabase().GetSource(),
	)
	if err != nil {
		return nil, err
	}
	// only a wrapper of the pg connection.
	pgCfg := postgres.Config{Conn: postgresConn}
	gormClient, err := gorm.Open(postgres.New(pgCfg), &gorm.Config{})
	if err != nil {
		_ = postgresConn.Close()
		return nil, err
	}
	return &Postgres{
		Conn:       postgresConn,
		GormClient: gormClient,
	}, nil
}
