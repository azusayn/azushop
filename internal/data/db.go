package data

import (
	"azushop/internal/biz"
	"context"
	"errors"

	"gorm.io/gorm"
)

type Transaction struct {
	data *Data
}

func NewTransaction(data *Data) biz.Transaction {
	return &Transaction{data: data}
}

type ContextKey int

// TODO(2): context key value.
// 101 ~ 200
const (
	TransactionCtxKey = 101
)

func (tx *Transaction) Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if tx.data == nil {
		return errors.New("nil data")
	}
	// NOTES: this can be replaced with gorm.Transaction() API,
	// but deeply nested closures hurt readability.
	gormClient := tx.data.gormClient
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

func GetTransaction(ctx context.Context) *gorm.DB {
	tx, ok := ctx.Value(TransactionCtxKey).(*gorm.DB)
	if !ok || tx == nil {
		return tx
	}
	return tx
}
