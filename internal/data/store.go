package data

import (
	"azushop/internal/biz"
	"azushop/internal/conf"
	"context"
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

type ContextKey int

// TODO(2): context key value.
// 101 ~ 200
const (
	TransactionCtxKey = 101
)

type Transaction struct {
	postgres *Postgres
}

func NewTransaction(postgres *Postgres) biz.Transaction {
	return &Transaction{postgres: postgres}
}

func (tx *Transaction) Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if tx.postgres == nil {
		return errors.New("database client is nil")
	}
	// NOTES: this can be replaced with gorm.Transaction() API,
	// but deeply nested closures hurt readability.
	gormClient := tx.postgres.GormClient
	if gormClient == nil {
		return errors.New("orm client is nil")
	}
	gormClient = gormClient.WithContext(ctx).Begin()
	defer gormClient.Rollback()
	ctx = context.WithValue(ctx, TransactionCtxKey, gormClient)
	if err := fn(ctx); err != nil {
		return err
	}
	return gormClient.Commit().Error
}

func GetTransaction(ctx context.Context) *gorm.DB {
	tx, ok := ctx.Value(TransactionCtxKey).(*gorm.DB)
	if !ok || tx == nil {
		return tx
	}
	return tx
}
