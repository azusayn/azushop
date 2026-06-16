package middleware

import (
	"context"
	"log/slog"

	"github.com/go-kratos/kratos/v2/middleware"
)

func LogInterceptor() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			resp, err := handler(ctx, req)
			if err != nil {
				slog.Error("request failed", slog.Any("error", err))
				return nil, err
			}
			slog.Info("request succeeded", slog.Any("response", resp))
			return resp, err
		}
	}
}
