package store

import (
	"context"
	"errors"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrInvalidStatus = errors.New("invalid status")
)

type CreateUserParams struct {
	Username     string
	Email        string
	PasswordHash string
}

type CreatePostParams struct {
	UserID         int64
	Caption        string
	IdempotencyKey string
	Media          []CreateMediaParams
}

type CreateMediaParams struct {
	Position    int
	Type        MediaType
	ContentType string
	Size        int64
}

type FeedCursor struct {
	CreatedAtUnixNano int64 `json:"created_at_unix_nano"`
	ID                int64 `json:"id"`
}

type Repository interface {
	CreateUser(ctx context.Context, params CreateUserParams) (User, error)
	FindUserByEmail(ctx context.Context, email string) (User, error)
	FindUserByID(ctx context.Context, id int64) (User, error)

	CreatePost(ctx context.Context, params CreatePostParams) (PostWithMedia, error)
	GetPost(ctx context.Context, postID int64) (PostWithMedia, error)
	ListFeed(ctx context.Context, cursor *FeedCursor, limit int) ([]PostWithMedia, *FeedCursor, error)

	MarkMediaProcessing(ctx context.Context, mediaID int64) (Media, error)
	MarkMediaReady(ctx context.Context, mediaID int64, updates Media) (Media, error)
	MarkMediaFailed(ctx context.Context, mediaID int64, reason string) (Media, error)
	CompletePostIfReady(ctx context.Context, postID int64) (Post, error)

	LikePost(ctx context.Context, userID, postID int64) (PostStats, error)
	UnlikePost(ctx context.Context, userID, postID int64) (PostStats, error)
	AddComment(ctx context.Context, userID, postID int64, body string) (Comment, PostStats, error)
	IncrementView(ctx context.Context, postID int64) (PostStats, error)
}
