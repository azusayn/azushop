package service

import (
	"context"
	"crypto/ed25519"

	pb "github.com/azusayn/azushop/proto/api/auth/v1"
	"github.com/azusayn/azushop/proto/conf"

	"github.com/azusayn/azushop/internal/biz"
	"github.com/azusayn/azushop/internal/pkg/crypto"

	"connectrpc.com/connect"
)

type AuthService struct {
	pb.UnimplementedAuthServiceServer
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
	token, err := h.authService.uc.Login(ctx, h.authService.privateKey, h.authService.issuer, req.Msg.Name, req.Msg.Password, h.authService.keyVersion)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.LoginResponse{
		AccessToken: token,
	}), nil
}

func (h *AuthServiceConnectHandler) Register(ctx context.Context, req *connect.Request[pb.RegisterRequest]) (*connect.Response[pb.RegisterResponse], error) {
	if err := h.authService.uc.Register(ctx, req.Msg.Name, req.Msg.Password); err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.RegisterResponse{}), nil
}
