package service

import (
	"context"

	pb "azushop/api/auth/v1"
)

type AuthServiceService struct {
	pb.UnimplementedAuthServiceServer
}

func NewAuthServiceService() *AuthServiceService {
	return &AuthServiceService{}
}

func (s *AuthServiceService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	return &pb.LoginResponse{
		AccessToken: "Fuck",
	}, nil
}
func (s *AuthServiceService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	return &pb.RegisterResponse{}, nil
}
