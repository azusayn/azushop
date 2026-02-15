package service

import (
	pb "azushop/api/auth/v1"
	"azushop/internal/biz"
	"context"
)

type AuthServiceService struct {
	pb.UnimplementedAuthServiceServer
	uc *biz.UserUsecase
}

func NewAuthServiceService(uc *biz.UserUsecase) *AuthServiceService {
	return &AuthServiceService{uc: uc}
}

func (s *AuthServiceService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	return &pb.LoginResponse{}, nil
}

func (s *AuthServiceService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if err := s.uc.Register(ctx, req.Name, req.Password); err != nil {
		return nil, err
	}
	return &pb.RegisterResponse{}, nil
}
