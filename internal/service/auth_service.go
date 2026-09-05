package service

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"

	pb "github.com/azusayn/azushop/proto/api/auth/v1"
	"github.com/azusayn/azushop/proto/conf"

	"github.com/azusayn/azushop/internal/biz"
	"github.com/azusayn/azushop/internal/pkg/crypto"

	"connectrpc.com/connect"
)

type AuthService struct {
	uc         *biz.UserUsecase
	privateKey ed25519.PrivateKey
	issuer     string
	keyVersion string
}

func NewAuthService(uc *biz.UserUsecase, config *conf.Data) (*AuthService, error) {
	path := config.GetAuth().GetPrivateKeyPath()
	privateKey, err := crypto.LoadEd25519PrivateKey(path)
	if err != nil {
		return nil, err
	}
	return &AuthService{
		uc:         uc,
		privateKey: privateKey,
		issuer:     config.GetAuth().GetIssuer(),
		keyVersion: config.GetAuth().GetKeyVersion(),
	}, nil
}

// AuthServiceConnectHandler implements the ConnectRPC handler for AuthService.
type AuthServiceConnectHandler struct {
	authService *AuthService
}

func NewAuthServiceConnectHandler(authService *AuthService) *AuthServiceConnectHandler {
	return &AuthServiceConnectHandler{authService: authService}
}

func (h *AuthServiceConnectHandler) Login(ctx context.Context, req *connect.Request[pb.LoginRequest]) (*connect.Response[pb.LoginResponse], error) {
	msg := req.Msg
	switch msg.GetIdentityProvider() {
	case pb.IdentityProvider_PROVIDER_LOCAL:
		providerCtx := msg.GetIdentityProviderContext()
		if providerCtx == nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("identity_provider_context is required"))
		}
		switch providerCtx.GetContext().(type) {
		case *pb.IdentityProviderContext_PasswordContext:
			name, password, err := resolvePasswordContext(providerCtx.GetPasswordContext())
			if err != nil {
				return nil, err
			}
			token, err := h.authService.uc.Login(
				ctx,
				h.authService.privateKey,
				h.authService.issuer,
				name,
				password,
				h.authService.keyVersion,
			)
			if err != nil {
				return nil, err
			}
			return connect.NewResponse(&pb.LoginResponse{
				AccessToken: token,
			}), nil
		case *pb.IdentityProviderContext_Oauth2Context,
			*pb.IdentityProviderContext_OtpContext,
			*pb.IdentityProviderContext_OidcContext:
			return nil, connect.NewError(connect.CodeUnimplemented, errors.New("local login context is not implemented"))
		default:
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("identity_provider_context.context is required"))
		}
	case pb.IdentityProvider_PROVIDER_GOOGLE, pb.IdentityProvider_PROVIDER_GITHUB:
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("identity provider %s is not implemented", msg.GetIdentityProvider()))
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("identity_provider is required"))
	}
}

func (h *AuthServiceConnectHandler) Register(ctx context.Context, req *connect.Request[pb.RegisterRequest]) (*connect.Response[pb.RegisterResponse], error) {
	local := req.Msg.GetLocalIdentityContext()
	if local == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("local_identity_context is required"))
	}
	switch local.GetContext().(type) {
	case *pb.LocalIdentityContext_PasswordContext:
		name, password, err := resolvePasswordContext(local.GetPasswordContext())
		if err != nil {
			return nil, err
		}
		if err := h.authService.uc.Register(ctx, name, password); err != nil {
			return nil, err
		}
		return connect.NewResponse(&pb.RegisterResponse{}), nil
	case *pb.LocalIdentityContext_OtpContext:
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("otp registration is not implemented"))
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("local_identity_context.context is required"))
	}
}

func resolvePasswordContext(pc *pb.PasswordContext) (name, password string, err error) {
	if pc == nil {
		return "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("password_context is required"))
	}
	password = pc.GetPassword()
	if password == "" {
		return "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("password is required"))
	}
	switch pc.GetName().(type) {
	case *pb.PasswordContext_Username:
		name = pc.GetUsername()
		if name == "" {
			return "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("username is required"))
		}
		return name, password, nil
	case *pb.PasswordContext_Email:
		return "", "", connect.NewError(connect.CodeUnimplemented, errors.New("email password auth is not implemented"))
	default:
		return "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("username or email is required"))
	}
}
