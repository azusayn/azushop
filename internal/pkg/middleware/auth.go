package middleware

import (
	v1 "azushop/api/auth/v1"
	"azushop/internal/common"
	"context"
	"strings"

	"github.com/azusayn/azutils/auth"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func AuthInterceptor(publicKey any, issuer string) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return nil, status.Error(codes.Internal, codes.Internal.String())
			}
			// ei wtf is wrong with kratos, I can't extract metadata from it.
			if requireAuth(tr.Operation()) {
				md, ok := metadata.FromIncomingContext(ctx)
				if !ok {
					return nil, status.Error(codes.Unauthenticated, codes.Unauthenticated.String())
				}
				vals := md.Get(auth.HttpHeaderAuthorization)
				if len(vals) == 0 {
					return nil, status.Error(codes.Unauthenticated, "missing token")
				}
				tokens := strings.Split(vals[0], " ")
				if len(tokens) != 2 || strings.ToLower(tokens[0]) != auth.HttpHeaderBearer {
					return nil, status.Error(codes.Unauthenticated, "invalid access token format")
				}
				userID, role, err := auth.ValidateAccessToken(publicKey, tokens[1], issuer)
				if err != nil {
					return nil, status.Error(codes.Unauthenticated, err.Error())
				}
				common.WithUserInfo(&ctx, userID, role)
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
