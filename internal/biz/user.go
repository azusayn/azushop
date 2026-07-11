package biz

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"time"

	azuerr "github.com/azusayn/azushop/internal/common/errors"

	"github.com/azusayn/azutils/validate"
	"github.com/golang-jwt/jwt/v5"

	"github.com/azusayn/azutils/auth"
	"github.com/azusayn/azutils/crypto"
)

type UserRole string

const (
	UserRoleCustomer      UserRole = "customer"
	UserRoleMerchant      UserRole = "merchant"
	UserRoleAdministrator UserRole = "admin"
)

type User struct {
	ID           int32
	Name         string
	PasswordHash string
	Salt         string
	Role         UserRole
}

type UserRepo interface {
	FindByName(context.Context, string) (*User, error)
	Save(context.Context, *User) error
}

type UserUsecase struct {
	repo UserRepo
}

func NewUserUsecase(repo UserRepo) *UserUsecase {
	return &UserUsecase{repo: repo}
}

// TODO: error code design
func (uc *UserUsecase) Register(ctx context.Context, name, password string) error {
	if err := validate.CheckUsername(name); err != nil {
		return err
	}
	if _, err := uc.repo.FindByName(ctx, name); err == nil {
		return errors.New(string(azuerr.MsgUsernameAlreadyExist))
	}

	salt := crypto.GenerateRandomHexString(16)
	passwordHash := crypto.Sha256(salt, password)

	return uc.repo.Save(ctx, &User{
		Name:         name,
		PasswordHash: passwordHash,
		Salt:         salt,
		// TODO: merchant register interface.
		Role: UserRoleCustomer,
	})
}

func (uc *UserUsecase) Login(
	ctx context.Context,
	privateKey ed25519.PrivateKey,
	issuer string,
	name string,
	password string,
	version string,
) (string, error) {
	if err := validate.CheckUsername(name); err != nil {
		return "", err
	}
	user, err := uc.repo.FindByName(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New(string(azuerr.MsgUserNotExist))
		}
		return "", err
	}
	// TODO: Argon2
	passwordHash := crypto.Sha256(user.Salt, password)
	if passwordHash != user.PasswordHash {
		return "", errors.New(string(azuerr.MsgInvalidUsernameOrPassword))
	}
	return auth.GenerateAccessToken(
		jwt.SigningMethodEdDSA,
		privateKey,
		issuer,
		// TODO: dev
		time.Minute*1024*1024,
		version,
		user.ID,
		string(user.Role),
	)
}
