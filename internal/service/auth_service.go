package service

import (
	pb "azushop/api/auth/v1"
	"azushop/internal/biz"
	"azushop/internal/data"
	"context"
	"crypto/rsa"
)

type AuthServiceService struct {
	pb.UnimplementedAuthServiceServer
	uc         *biz.UserUsecase
	authConfig *AuthConfig
}

// TODO: move this to proper place.
type AuthConfig struct {
	appName    string
	privateKey *rsa.PrivateKey
}

func NewAuthConfig(data *data.Data) *AuthConfig {
	return &AuthConfig{
		data.AppName,
		data.PrivateKey,
	}
}
func NewAuthServiceService(uc *biz.UserUsecase, authConfig *AuthConfig) *AuthServiceService {
	return &AuthServiceService{
		uc:         uc,
		authConfig: authConfig,
	}
}

func (s *AuthServiceService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	token, err := s.uc.Login(
		ctx,
		s.authConfig.privateKey,
		s.authConfig.appName,
		req.Name,
		req.Password,
	)
	if err != nil {
		return nil, err
	}
	return &pb.LoginResponse{
		AccessToken: token,
	}, nil
}

func (s *AuthServiceService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if err := s.uc.Register(ctx, req.Name, req.Password); err != nil {
		return nil, err
	}
	return &pb.RegisterResponse{}, nil
}
