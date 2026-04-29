package service

import (
	pb "azushop/api/auth/v1"
	"azushop/internal/biz"
	"azushop/internal/conf"
	"context"
	"crypto/rsa"

	"github.com/azusayn/azutils/auth"
)

type AuthService struct {
	pb.UnimplementedAuthServiceServer
	uc         *biz.UserUsecase
	privateKey *rsa.PrivateKey
	appName    string
}

func NewAuthService(uc *biz.UserUsecase, config *conf.Data) *AuthService {
	privateKey, err := auth.GeneratePrivateKey()
	if err != nil {
		panic("failed to init server secret")
	}
	return &AuthService{
		uc:         uc,
		privateKey: privateKey,
		appName:    config.AppName,
	}
}

func (s *AuthService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	token, err := s.uc.Login(ctx, s.privateKey, s.appName, req.Name, req.Password)
	if err != nil {
		return nil, err
	}
	return &pb.LoginResponse{
		AccessToken: token,
	}, nil
}

func (s *AuthService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if err := s.uc.Register(ctx, req.Name, req.Password); err != nil {
		return nil, err
	}
	return &pb.RegisterResponse{}, nil
}
