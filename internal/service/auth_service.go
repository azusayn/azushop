package service

import (
	pb "azushop/api/auth/v1"
	"azushop/internal/biz"
	"azushop/internal/conf"
	"context"
	"crypto/rsa"

	"github.com/azusayn/azutils/auth"
)

var secret *rsa.PrivateKey

type AuthServiceService struct {
	pb.UnimplementedAuthServiceServer
	uc         *biz.UserUsecase
	privateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
	AppName    string
}

func NewAuthServiceService(uc *biz.UserUsecase, config *conf.Data) *AuthServiceService {
	privateKey, err := auth.GeneratePrivateKey()
	if err != nil {
		panic("failed to init server secret")
	}
	return &AuthServiceService{
		uc:         uc,
		privateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
		AppName:    config.AppName,
	}
}

func (s *AuthServiceService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	token, err := s.uc.Login(ctx, s.privateKey, s.AppName, req.Name, req.Password)
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
