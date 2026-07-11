package middleware

import (
	"context"
	"strconv"
	"strings"

	v1 "github.com/azusayn/azushop/proto/api/auth/v1"

	"github.com/azusayn/azushop/internal/common"
	"github.com/azusayn/azutils/auth"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func AuthInterceptor(publicKey any, issuer string, verify bool) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return nil, status.Error(codes.Internal, codes.Internal.String())
			}
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

				jwToken := tokens[1]

				var userID int32
				var role string

				if verify {
					var err error
					userID, role, err = auth.ValidateAccessToken(publicKey, jwToken, issuer)
					if err != nil {
						return nil, status.Error(codes.Unauthenticated, err.Error())
					}
				} else {
					// NOTE: JWT token pass-through
					parser := jwt.NewParser()
					// TODO: move it to utils
					claim := auth.CustomClaims{}

					_, _, err := parser.ParseUnverified(jwToken, &claim)
					if err != nil {
						return nil, status.Error(codes.InvalidArgument, err.Error())
					}

					sub, err := claim.GetSubject()
					if err != nil {
						return nil, err
					}
					id, err := strconv.ParseInt(sub, 10, 32)
					if err != nil {
						return nil, err
					}
					userID = int32(id)

					if role, err = claim.GetRole(); err != nil {
						return nil, err
					}
				}

				common.WithUserInfo(&ctx, userID, role)
				bearerToken := auth.HttpHeaderBearer + " " + jwToken
				common.WithServiceInnerToken(&ctx, bearerToken)
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
