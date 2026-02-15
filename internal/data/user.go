package data

import (
	"azushop/internal/biz"
	"context"
	"database/sql"
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
	stmt := "select (id, name, password_hash, salt) from user where name=$1"
	row := client.QueryRowContext(ctx, stmt, name)
	if err := row.Scan(&user.ID, &user.Name, &user.PasswordHash, &user.Salt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (repo *UserRepo) Save(ctx context.Context, user *biz.User) error {
	client := repo.data.postgresClient
	stmt := "insert into user(name, password_hash, salt) values($1, $2, $3)"
	_, err := client.ExecContext(ctx, stmt, user.Name, user.PasswordHash, user.Salt)
	return err
}
