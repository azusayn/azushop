package middleware

import (
	"azushop/internal/common"
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// AuthClientInterceptor returns a client interceptor that enables transparent
// token pass-through from the client to downstream services.
// DO NOT use this in production.
func AuthClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption) error {
		token, err := common.ExtractServiceInnerToken(ctx)
		if err != nil {
			return err
		}
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", token)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
