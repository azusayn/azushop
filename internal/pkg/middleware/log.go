package middleware

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
)

func LogInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			if err != nil {
				slog.Error("request failed", slog.Any("error", err))
				return nil, err
			}
			slog.Info("request succeeded", slog.String("procedure", req.Spec().Procedure))
			return resp, err
		}
	}
}
