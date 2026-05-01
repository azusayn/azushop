package service

import (
	pb "azushop/api/auth/v1"
	"azushop/internal/biz"
	"azushop/internal/conf"
	"azushop/internal/pkg/crypto"
	"context"
	"crypto/ed25519"
)

type AuthService struct {
	pb.UnimplementedAuthServiceServer
	uc         *biz.UserUsecase
	privateKey ed25519.PrivateKey
	appName    string
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
		appName:    config.GetAppName(),
		keyVersion: config.GetAuth().GetKeyVersion(),
	}, nil
}

func (s *AuthService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	token, err := s.uc.Login(ctx, s.privateKey, s.appName, req.Name, req.Password, s.keyVersion)
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
