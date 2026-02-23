package data

import (
	"azushop/internal/biz"
	"context"
)

type UserRepo struct {
	data *Data
}

func NewUserRepo(data *Data) biz.UserRepo {
	return &UserRepo{
		data: data,
	}
}

func (repo *UserRepo) FindByName(ctx context.Context, name string) (*biz.User, error) {
	client := repo.data.postgresClient
	var user biz.User
	stmt := "select id, username, password_hash, salt from users where username=$1"
	row := client.QueryRowContext(ctx, stmt, name)
	if err := row.Scan(&user.ID, &user.Name, &user.PasswordHash, &user.Salt, &user.Role); err != nil {
		return nil, err
	}
	return &user, nil
}

func (repo *UserRepo) Save(ctx context.Context, user *biz.User) error {
	client := repo.data.postgresClient
	stmt := "insert into users(username, password_hash, salt, role) values($1, $2, $3, $4)"
	_, err := client.ExecContext(ctx, stmt, user.Name, user.PasswordHash, user.Salt, user.Role)
	return err
}
