package biz

import (
	"context"
	"fmt"

	"github.com/azusayn/azutils/auth"
	"github.com/azusayn/azutils/crypto"
)

type User struct {
	ID           int64
	Name         string
	PasswordHash string
	Salt         string
}

type UserRepo interface {
	FindByName(context.Context, string) (*User, error)
	Save(context.Context, *User) error
}

type UserUsecase struct {
	repo UserRepo
}

func NewUserUsercase(repo UserRepo) *UserUsecase {
	return &UserUsecase{repo: repo}
}

// TODO: error code design
func (uc *UserUsecase) Register(ctx context.Context, name, password string) error {
	if _, err := uc.repo.FindByName(ctx, name); err == nil {
		return fmt.Errorf("username %q exists", name)
	}

	if err := auth.CheckUsername(name); err != nil {
		return err
	}

	salt := crypto.GenerateRandomBytes(16)
	passwordHash := crypto.Sha256(password, salt)

	if err := uc.repo.Save(ctx, &User{
		Name:         name,
		PasswordHash: passwordHash,
		Salt:         salt,
	}); err != nil {
		return err
	}

	return nil
}
