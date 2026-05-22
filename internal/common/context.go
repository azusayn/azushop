package common

import (
	"context"
	"errors"
)

type ContextKey int

const (
	// 0 - 1000
	ServiceInnerTokenKey ContextKey = 0

	// 1000+
	UserIDCtxKey   ContextKey = 1001
	UserRoleCtxKey ContextKey = 1002

	// 1100+
	TransactionCtxKey ContextKey = 1101
)

func WithServiceInnerToken(ctx *context.Context, token string) {
	*ctx = context.WithValue(*ctx, ServiceInnerTokenKey, token)
}

func ExtractServiceInnerToken(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", errors.New("missing context")
	}
	token, ok := ctx.Value(ServiceInnerTokenKey).(string)
	if !ok || token == "" {
		return "", errors.New("missing inner token")
	}
	return token, nil
}

// append user id & user role to the ctx.
func WithUserInfo(ctx *context.Context, ID int32, role string) {
	*ctx = context.WithValue(*ctx, UserIDCtxKey, ID)
	*ctx = context.WithValue(*ctx, UserRoleCtxKey, role)
}

// extract user id & user role from the ctx.
func ExtractUserInfo(ctx *context.Context) (int32, string, error) {
	id, ok := (*ctx).Value(UserIDCtxKey).(int32)
	if !ok {
		return 0, "", errors.New("failed to extract user id")
	}
	role, ok := (*ctx).Value(UserRoleCtxKey).(string)
	if !ok {
		return 0, "", errors.New("failed to extract user role")
	}
	return id, role, nil
}
