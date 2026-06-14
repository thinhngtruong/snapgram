package store

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type PostStatus string

const (
	PostStatusDraft  PostStatus = "draft"
	PostStatusReady  PostStatus = "ready"
	PostStatusFailed PostStatus = "failed"
)

type MediaType string

const (
	MediaTypeImage MediaType = "image"
	MediaTypeVideo MediaType = "video"
)

type MediaStatus string

const (
	MediaStatusUploading  MediaStatus = "uploading"
	MediaStatusProcessing MediaStatus = "processing"
	MediaStatusReady      MediaStatus = "ready"
	MediaStatusFailed     MediaStatus = "failed"
)

type Post struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	Caption   string     `json:"caption"`
	Status    PostStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
}

type Media struct {
	ID             int64             `json:"id"`
	PostID         int64             `json:"post_id"`
	Position       int               `json:"position"`
	Type           MediaType         `json:"type"`
	Status         MediaStatus       `json:"status"`
	OriginalKey    string            `json:"s3_key_original"`
	VariantKeys    map[string]string `json:"s3_key_variants,omitempty"`
	HLSManifestKey string            `json:"hls_manifest_key,omitempty"`
	PosterKey      string            `json:"poster_key,omitempty"`
	Width          int               `json:"width,omitempty"`
	Height         int               `json:"height,omitempty"`
	DurationMS     int               `json:"duration_ms,omitempty"`
	Error          string            `json:"error,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
}

type PostStats struct {
	PostID       int64 `json:"post_id"`
	LikeCount    int64 `json:"like_count"`
	CommentCount int64 `json:"comment_count"`
	ViewCount    int64 `json:"view_count"`
}

type Comment struct {
	ID        int64     `json:"id"`
	PostID    int64     `json:"post_id"`
	UserID    int64     `json:"user_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type PostWithMedia struct {
	Post     Post      `json:"post"`
	Media    []Media   `json:"media"`
	Stats    PostStats `json:"stats"`
	Comments []Comment `json:"comments,omitempty"`
}
