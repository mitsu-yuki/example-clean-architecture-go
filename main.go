package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// entity
type User struct {
	id         int
	name       string
	email      string
	statusCode int
}

func NewUser(id int, name string, email string, statusCode int) (*User, error) {
	if id < 1 {
		return nil, errors.New("id must be greater than 1")
	}
	if name == "" {
		return nil, errors.New("name must not empty")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, err
	}
	return &User{
		id:         id,
		name:       name,
		email:      email,
		statusCode: statusCode,
	}, nil
}

func (u User) ID() int         { return u.id }
func (u User) Name() string    { return u.name }
func (u User) Email() string   { return u.email }
func (u User) StatusCode() int { return u.statusCode }

// entity: data access interface
type FindUserRepository interface {
	FindAll(ctx context.Context) ([]*User, error)
}
type UploadUserRepository interface {
	Upload(ctx context.Context, user *User) error
}

// infrastructure
type PostgresFindUserRepository struct {
	db *sqlx.DB
}

func NewPostgresFindUserRepository(db *sqlx.DB) FindUserRepository {
	return &PostgresFindUserRepository{db: db}
}

type PostgresUser struct {
	Id         int    `db:"id"`
	Name       string `db:"name"`
	Email      string `db:"email"`
	StatusCode int    `db:"status_code"`
}

func (r PostgresFindUserRepository) FindAll(ctx context.Context) ([]*User, error) {
	query := `SELECT id, name, email, status_code FROM system.user`
	var pgUsers []PostgresUser
	if err := r.db.SelectContext(ctx, &pgUsers, query); err != nil {
		return nil, err
	}
	var users []*User
	for _, pgUser := range pgUsers {
		user, err := NewUser(pgUser.Id, pgUser.Name, pgUser.Email, pgUser.StatusCode)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

type S3UploadUserRepository struct {
	client    *s3.Client
	bucket    string
	keyPrefix string
}

func NewS3UploadUserRepository(client *s3.Client, bucket string, prefix string) UploadUserRepository {
	return &S3UploadUserRepository{client: client, bucket: bucket, keyPrefix: prefix}
}

type S3User struct {
	Id         int    `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	StatusCode int    `json:"status_code"`
}

func (r S3UploadUserRepository) Upload(ctx context.Context, user *User) error {
	s3User := S3User{
		Id:         user.ID(),
		Name:       user.Name(),
		Email:      user.Email(),
		StatusCode: user.StatusCode(),
	}
	data, err := json.MarshalIndent(s3User, "", "  ")
	if err != nil {
		return err
	}
	_, err = r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(fmt.Sprintf("%s/user-%d.json", r.keyPrefix, user.ID())),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return err
	}

	return nil
}

// usecase dto (I/O boundary)
type UserDTO struct {
	ID         int
	Name       string
	Email      string
	StatusCode int
}

func userToDTO(u *User) *UserDTO {
	return &UserDTO{
		ID:         u.ID(),
		Name:       u.Name(),
		Email:      u.Email(),
		StatusCode: u.StatusCode(),
	}
}

func dtoToUser(dto *UserDTO) (*User, error) {
	return NewUser(dto.ID, dto.Name, dto.Email, dto.StatusCode)
}

// usecase
type FindAllUserUseCase struct{ repo FindUserRepository }

func NewFindAllUserUseCase(r FindUserRepository) *FindAllUserUseCase {
	return &FindAllUserUseCase{repo: r}
}

func (uc *FindAllUserUseCase) Run(ctx context.Context) ([]*UserDTO, error) {
	users, err := uc.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	var dtos []*UserDTO
	for _, u := range users {
		dtos = append(dtos, userToDTO(u))
	}
	return dtos, nil
}

type UploadUserUseCase struct {
	repo UploadUserRepository
}

func NewUploadUserUseCase(r UploadUserRepository) *UploadUserUseCase {
	return &UploadUserUseCase{repo: r}
}

func (uc *UploadUserUseCase) Run(ctx context.Context, dto *UserDTO) error {
	u, err := dtoToUser(dto)
	if err != nil {
		return err
	}
	return uc.repo.Upload(ctx, u)
}

func main() {
	ctx := context.Background()
	db, err := sqlx.Connect("postgres", "postgres://user@postgres.example.com/company")
	if err != nil {
		panic(err)
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		panic(err)
	}
	client := s3.NewFromConfig(cfg)

	pgRepo := NewPostgresFindUserRepository(db)
	s3Repo := NewS3UploadUserRepository(client, "company", "system/user")

	findAllUC := NewFindAllUserUseCase(pgRepo)
	uploadUC := NewUploadUserUseCase(s3Repo)

	dtos, err := findAllUC.Run(ctx)
	if err != nil {
		panic(err)
	}
	for _, dto := range dtos {
		if err := uploadUC.Run(ctx, dto); err != nil {
			panic(err)
		}
	}
}
