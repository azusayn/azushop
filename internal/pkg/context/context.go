package context

import (
	"context"
	"errors"
)

type ContextKey int

// 0 ~ 100
// TODO(2): context key value.
const (
	UserIDCtxKey   ContextKey = 0
	UserRoleCtxKey ContextKey = 1
)

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
