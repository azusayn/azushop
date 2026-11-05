package db

import (
	"context"
	"fmt"

	"github.com/azusayn/azushop/proto/conf"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pkg/errors"
	"github.com/uptrace/opentelemetry-go-extra/otelsql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	MaxDBOpenConnections = 20
)

type Client struct {
	// TODO: make it unexported.
	DB *gorm.DB
}

func NewClient(config *conf.Data) (*Client, error) {
	if config == nil {
		return nil, errors.New("missing database configurations")
	}

	// TODO: implement retry policy for database connection initialization.
	//
	// Since all the pods start almost at the same time, the database connection may fail if
	// the database pod is not ready. Instead of crashing and letting Kubernetes restart the
	// pod (which may cause backoff delays), restarting the application inside the pod with
	// short-interval retries is more efficient.

	driver := config.GetDatabase().GetDriver()
	sqlDB, err := otelsql.Open(
		driver,
		config.GetDatabase().GetSource(),
	)
	if err != nil {
		return nil, err
	}

	if v := config.Database.MaxOpenConnections; v == 0 {
		sqlDB.SetMaxOpenConns(MaxDBOpenConnections)
	}

	var dialector gorm.Dialector
	switch driver {
	case "pgx":
		dialector = postgres.New(postgres.Config{Conn: sqlDB})

	default:
		return nil, fmt.Errorf("unsupported database client driver %q", driver)
	}

	gormDB, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	return &Client{
		DB: gormDB,
	}, nil
}

type transactionContextKey string

const (
	txCtxKey transactionContextKey = "tx"
)

func (c *Client) Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	tx := c.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	ctx = context.WithValue(ctx, txCtxKey, tx)

	if err := fn(ctx); err != nil {
		return err
	}

	return tx.Commit().Error
}

func (c *Client) GetTransaction(ctx context.Context) *gorm.DB {
	tx, ok := ctx.Value(txCtxKey).(*gorm.DB)
	if !ok {
		return c.DB
	}
	return tx
}
