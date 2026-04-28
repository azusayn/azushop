package data

import (
	"azushop/internal/biz"
	"context"
	"errors"

	"gorm.io/gorm"
)

type TransactionV2 struct {
	postgres *Postgres
}

func NewTransactionV2(postgres *Postgres) biz.Transaction {
	return &TransactionV2{postgres: postgres}
}

func (tx *TransactionV2) Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if tx.postgres == nil {
		return errors.New("nil data")
	}
	// NOTES: this can be replaced with gorm.Transaction() API,
	// but deeply nested closures hurt readability.
	gormClient := tx.postgres.gormClient
	if gormClient == nil {
		return errors.New("nil gorm client")
	}
	gormClient = gormClient.WithContext(ctx).Begin()
	defer gormClient.Rollback()
	ctx = context.WithValue(ctx, TransactionCtxKey, gormClient)
	if err := fn(ctx); err != nil {
		return err
	}
	return gormClient.Commit().Error
}

func GetTransactionV2(ctx context.Context) *gorm.DB {
	tx, ok := ctx.Value(TransactionCtxKey).(*gorm.DB)
	if !ok || tx == nil {
		return tx
	}
	return tx
}
