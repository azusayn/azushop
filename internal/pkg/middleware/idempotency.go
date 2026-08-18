package middleware

import (
	"context"

	"connectrpc.com/connect"
	"github.com/azusayn/azushop/internal/common"
)

const (
	HttpHeaderIdempotencyKey string = "idempotency-key"
)

func IdempotencyInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			key := req.Header().Get(HttpHeaderIdempotencyKey)
			if key != "" {
				common.WithIdempotencyKey(&ctx, key)
			}
			return next(ctx, req)
		}

	}
}
