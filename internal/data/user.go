package data

import (
	"context"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"github.com/azusayn/azushop/internal/biz"
	"github.com/azusayn/azushop/internal/pkg/db"
	"github.com/azusayn/azushop/internal/pkg/grpcclient"
	authv1connect "github.com/azusayn/azushop/proto/api/auth/v1/v1connect"
	"github.com/azusayn/azushop/proto/conf"
	"github.com/google/wire"
	"go.opentelemetry.io/otel/propagation"
)

var AuthDataProviderSet = wire.NewSet(
	db.NewClient,
	NewUserRepo,
)

type UserRepo struct {
	db *db.Client
}

func NewUserRepo(db *db.Client) biz.UserRepo {
	return &UserRepo{
		db: db,
	}
}

func NewAuthClient(config *conf.Data, propagator propagation.TextMapPropagator) (authv1connect.AuthServiceClient, error) {
	otelInterceptor, err := otelconnect.NewInterceptor(
		otelconnect.WithPropagator(propagator),
	)
	if err != nil {
		return nil, err
	}
	return grpcclient.NewServiceClient(
		authv1connect.NewAuthServiceClient,
		config.GetServiceAddr().GetAuth(),
		[]connect.ClientOption{
			connect.WithInterceptors(otelInterceptor),
			connect.WithGRPC(),
		},
	), nil
}

func (repo *UserRepo) FindByName(ctx context.Context, name string) (*biz.User, error) {
	var user biz.User
	stmt := "select id, username, password_hash, salt, role from users where username=$1"
	row := repo.db.GetTransaction(ctx).WithContext(ctx).Raw(stmt, name).Row()
	if err := row.Scan(&user.ID, &user.Name, &user.PasswordHash, &user.Salt, &user.Role); err != nil {
		return nil, err
	}
	return &user, nil
}

func (repo *UserRepo) Save(ctx context.Context, user *biz.User) error {
	stmt := "insert into users(username, password_hash, salt, role) values($1, $2, $3, $4)"
	return repo.db.GetTransaction(ctx).WithContext(ctx).Exec(stmt, user.Name, user.PasswordHash, user.Salt, user.Role).Error
}
