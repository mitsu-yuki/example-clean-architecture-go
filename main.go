package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type Todo struct {
	userId    int
	id        int
	title     string
	completed bool
}

func NewTodo(userId int, id int, title string, completed bool) (*Todo, error) {
	if userId == 0 {
		return nil, errors.New("userId must not be 0")
	}
	if id == 0 {
		return nil, errors.New("id must not be 0")
	}
	if title == "" {
		return nil, errors.New("title can not be empty")
	}
	return &Todo{
		userId:    userId,
		id:        id,
		title:     title,
		completed: completed,
	}, nil
}
func (t Todo) UserId() int     { return t.userId }
func (t Todo) Id() int         { return t.id }
func (t Todo) Title() string   { return t.title }
func (t Todo) Completed() bool { return t.completed }

type TodoRepository interface {
	FindAll(ctx context.Context) ([]*Todo, error)
	FindById(ctx context.Context, id int) (*Todo, error)
	Create(ctx context.Context, todo Todo) error
}

type HttpTodoRepository struct{ BaseURL string }

func NewHttpTodoRepository(baseURL string) TodoRepository {
	return &HttpTodoRepository{BaseURL: baseURL}
}

type HttpTodo struct {
	UserId    int    `json:"userId"`
	Id        int    `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

func (r HttpTodoRepository) FindAll(ctx context.Context) ([]*Todo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.BaseURL+"/todos", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var httpTodos []HttpTodo
	if err := json.Unmarshal(b, &httpTodos); err != nil {
		return nil, err
	}
	var todos []*Todo
	for _, h := range httpTodos {
		t, err := NewTodo(h.UserId, h.Id, h.Title, h.Completed)
		if err != nil {
			return nil, err
		}
		todos = append(todos, t)
	}
	return todos, err
}

func (r HttpTodoRepository) FindById(ctx context.Context, id int) (*Todo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/todos/%d", r.BaseURL, id), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			panic(err)
		}
	}()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var h HttpTodo
	if err := json.Unmarshal(b, &h); err != nil {
		return nil, err
	}
	return NewTodo(h.UserId, h.Id, h.Title, h.Completed)
}

func (r HttpTodoRepository) Create(ctx context.Context, todo Todo) error {
	h := HttpTodo{
		UserId:    todo.userId,
		Id:        todo.id,
		Title:     todo.title,
		Completed: todo.completed,
	}
	b, err := json.Marshal(h)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.BaseURL+"/todos", bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return errors.New("http error")
}

type FindAllTodoUseCase struct{ repo TodoRepository }

func NewFindAllTodoUseCase(repo TodoRepository) FindAllTodoUseCase {
	return FindAllTodoUseCase{repo: repo}
}

func (uc FindAllTodoUseCase) Run(ctx context.Context) ([]*Todo, error) {
	todos, err := uc.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	var vtodos []*Todo
	for _, todo := range todos {
		v, err := NewTodo(todo.UserId(), todo.Id(), todo.Title(), todo.Completed())
		if err != nil {
			return nil, err
		}
		vtodos = append(vtodos, v)
	}
	return vtodos, nil
}

type FindByIdTodoUseCase struct{ repo TodoRepository }

func NewFindByIdTodoUseCase(repo TodoRepository) FindByIdTodoUseCase {
	return FindByIdTodoUseCase{repo: repo}
}

func (uc FindByIdTodoUseCase) Run(ctx context.Context, id int) (*Todo, error) {
	todo, err := uc.repo.FindById(ctx, id)
	if err != nil {
		return nil, err
	}
	return todo, nil
}

type CreateTodoUseCase struct{ repo TodoRepository }

func NewCreateTodoUseCase(repo TodoRepository) CreateTodoUseCase {
	return CreateTodoUseCase{repo: repo}
}

func (uc CreateTodoUseCase) Run(ctx context.Context, todo Todo) error {
	return uc.repo.Create(ctx, todo)
}

type CreateAllTodoUseCase struct{ repo TodoRepository }

func NewCreateAllTodoUseCase(repo TodoRepository) CreateAllTodoUseCase {
	return CreateAllTodoUseCase{repo: repo}
}
func (uc CreateAllTodoUseCase) Run(ctx context.Context, todos []Todo) error {
	for _, todo := range todos {
		if err := uc.repo.Create(ctx, todo); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	repo := NewHttpTodoRepository("https://jsonplaceholder.typicode.com")
	findAllUC := NewFindAllTodoUseCase(repo)
	todos, err := findAllUC.Run(ctx)
	if err != nil {
		panic(err)
	}

	for _, t := range todos {
		fmt.Printf("%#v\n", t)
	}
}
