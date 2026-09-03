package middleware

import (
	"context"
	"crypto/ed25519"
	"strconv"
	"strings"

	"github.com/azusayn/azushop/internal/common"

	"github.com/azusayn/azutils/auth"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"connectrpc.com/connect"
)

// AuthInterceptor returns a ConnectRPC interceptor that extracts JWT claims
// from the Authorization header and injects user info into the request context.
func AuthInterceptor(publicKey ed25519.PublicKey, issuer string, verify bool) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Server-side: extract token from request headers
			token := req.Header().Get(auth.HttpHeaderAuthorization)
			if token == "" {
				return nil, status.Error(codes.Unauthenticated, "missing token")
			}
			parts := strings.Split(token, " ")
			if len(parts) != 2 || strings.ToLower(parts[0]) != auth.HttpHeaderBearer {
				return nil, status.Error(codes.Unauthenticated, "invalid access token format")
			}
			jwToken := parts[1]

			var userID int32
			var role string

			if verify {
				var err error
				userID, role, err = auth.ValidateAccessToken(publicKey, jwToken, issuer)
				if err != nil {
					return nil, status.Error(codes.Unauthenticated, err.Error())
				}
			} else {
				parser := jwt.NewParser()
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
			return next(ctx, req)
		}
	}
}

// AuthClientInterceptor returns a ConnectRPC client interceptor that forwards
// the internal service token from context to outgoing request headers.
func AuthClientInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			token, err := common.ExtractServiceInnerToken(ctx)
			if err != nil {
				return nil, err
			}
			req.Header().Set("authorization", token)
			return next(ctx, req)
		}
	}
}
