package middleware

import (
	v1 "azushop/api/auth/v1"
	"context"
	"crypto/rsa"
	"strings"

	"github.com/azusayn/azutils/auth"
	"github.com/go-kratos/kratos/v2/metadata"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func AuthInterceptor(publicKey *rsa.PublicKey, issuer string) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return nil, status.Error(codes.Internal, codes.Internal.String())
			}
			if requireAuth(tr.Operation()) {
				md, ok := metadata.FromServerContext(ctx)
				if !ok {
					return nil, status.Error(codes.Unauthenticated, codes.Unauthenticated.String())
				}
				val := md.Get(auth.HttpHeaderAuthorization)
				tokens := strings.Split(val, " ")
				if len(tokens) != 2 || strings.ToLower(tokens[0]) != auth.HttpHeaderBearer {
					return nil, status.Error(codes.Unauthenticated, "invalid access token format")
				}
				userID, role, err := auth.ValidateAccessToken(tokens[1], publicKey, issuer)
				if err != nil {
					return nil, status.Error(codes.Unauthenticated, err.Error())
				}
				WithUserInfo(&ctx, userID, role)
			}
			return handler(ctx, req)
		}
	}
}

// TODO: differ roles in different APIs.
func requireAuth(methodName string) bool {
	switch methodName {
	case v1.OperationAuthServiceLogin,
		v1.OperationAuthServiceRegister:
		return false
	default:
	}
	return true
}
