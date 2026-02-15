package biz

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

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
	if err := auth.CheckUsername(name); err != nil {
		return err
	}
	if _, err := uc.repo.FindByName(ctx, name); err == nil {
		return fmt.Errorf("username %q exists", name)
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

func (uc *UserUsecase) Login(
	ctx context.Context,
	privateKey *rsa.PrivateKey,
	issuer string,
	name string,
	password string,
) (string, error) {
	if err := auth.CheckUsername(name); err != nil {
		return "", err
	}

	user, err := uc.repo.FindByName(ctx, name)
	if err != nil {
		return "", err
	}

	passwordHash := crypto.Sha256(password, user.Salt)
	if passwordHash != user.PasswordHash {
		return "", errors.New("invalid username or password")
	}

	token, err := auth.GenerateAccessToken(user.ID, privateKey, issuer, time.Minute*15)
	if err != nil {
		return "", err
	}
	return token, nil
}
