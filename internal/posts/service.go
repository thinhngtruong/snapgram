package posts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/thinhnguyen/snapgram/internal/store"
)

var ErrInvalidPost = errors.New("invalid post")

type Service struct {
	repo store.Repository
}

func NewService(repo store.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, userID int64, caption, idempotencyKey string, media []store.CreateMediaParams) (store.PostWithMedia, error) {
	if len(media) < 1 || len(media) > 10 {
		return store.PostWithMedia{}, ErrInvalidPost
	}
	for i := range media {
		if media[i].Type != store.MediaTypeImage && media[i].Type != store.MediaTypeVideo {
			return store.PostWithMedia{}, ErrInvalidPost
		}
		media[i].Position = i + 1
	}
	return s.repo.CreatePost(ctx, store.CreatePostParams{
		UserID:         userID,
		Caption:        strings.TrimSpace(caption),
		IdempotencyKey: idempotencyKey,
		Media:          media,
	})
}

func (s *Service) Get(ctx context.Context, postID int64) (store.PostWithMedia, error) {
	return s.repo.GetPost(ctx, postID)
}

func (s *Service) Feed(ctx context.Context, cursorToken string, limit int) ([]store.PostWithMedia, string, error) {
	if limit < 1 || limit > 50 {
		limit = 20
	}
	cursor, err := decodeCursor(cursorToken)
	if err != nil {
		return nil, "", err
	}
	items, next, err := s.repo.ListFeed(ctx, cursor, limit)
	if err != nil || next == nil {
		return items, "", err
	}
	token, err := encodeCursor(*next)
	return items, token, err
}

func (s *Service) Like(ctx context.Context, userID, postID int64) (store.PostStats, error) {
	return s.repo.LikePost(ctx, userID, postID)
}

func (s *Service) Unlike(ctx context.Context, userID, postID int64) (store.PostStats, error) {
	return s.repo.UnlikePost(ctx, userID, postID)
}

func (s *Service) Comment(ctx context.Context, userID, postID int64, body string) (store.Comment, store.PostStats, error) {
	if strings.TrimSpace(body) == "" {
		return store.Comment{}, store.PostStats{}, ErrInvalidPost
	}
	return s.repo.AddComment(ctx, userID, postID, body)
}

func (s *Service) View(ctx context.Context, postID int64) (store.PostStats, error) {
	return s.repo.IncrementView(ctx, postID)
}

func encodeCursor(cursor store.FeedCursor) (string, error) {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(token string) (*store.FeedCursor, error) {
	if token == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, ErrInvalidPost
	}
	var cursor store.FeedCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return nil, ErrInvalidPost
	}
	return &cursor, nil
}
