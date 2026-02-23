package service

import (
	pb "azushop/api/auth/v1"
	"azushop/internal/biz"
	"azushop/internal/data"
	"context"
)

type AuthServiceService struct {
	pb.UnimplementedAuthServiceServer
	uc   *biz.UserUsecase
	data *data.Data
}

func NewAuthServiceService(uc *biz.UserUsecase, data *data.Data) *AuthServiceService {
	return &AuthServiceService{
		uc:   uc,
		data: data,
	}
}

func (s *AuthServiceService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	token, err := s.uc.Login(ctx, s.data.GetPrivateKey(), s.data.GetAppName(), req.Name, req.Password)
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
