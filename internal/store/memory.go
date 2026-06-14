package store

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type MemoryRepository struct {
	mu              sync.RWMutex
	nextUserID      int64
	nextPostID      int64
	nextMediaID     int64
	nextCommentID   int64
	users           map[int64]User
	usersByEmail    map[string]int64
	posts           map[int64]Post
	media           map[int64]Media
	mediaByPost     map[int64][]int64
	stats           map[int64]PostStats
	likes           map[int64]map[int64]bool
	commentsByPost  map[int64][]Comment
	idempotencyKeys map[string]int64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		users:           map[int64]User{},
		usersByEmail:    map[string]int64{},
		posts:           map[int64]Post{},
		media:           map[int64]Media{},
		mediaByPost:     map[int64][]int64{},
		stats:           map[int64]PostStats{},
		likes:           map[int64]map[int64]bool{},
		commentsByPost:  map[int64][]Comment{},
		idempotencyKeys: map[string]int64{},
	}
}

func (r *MemoryRepository) CreateUser(_ context.Context, params CreateUserParams) (User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	email := strings.ToLower(strings.TrimSpace(params.Email))
	if _, ok := r.usersByEmail[email]; ok {
		return User{}, ErrConflict
	}
	r.nextUserID++
	user := User{
		ID:           r.nextUserID,
		Username:     strings.TrimSpace(params.Username),
		Email:        email,
		PasswordHash: params.PasswordHash,
		CreatedAt:    time.Now().UTC(),
	}
	r.users[user.ID] = user
	r.usersByEmail[email] = user.ID
	return user, nil
}

func (r *MemoryRepository) FindUserByEmail(_ context.Context, email string) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.usersByEmail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return User{}, ErrNotFound
	}
	return r.users[id], nil
}

func (r *MemoryRepository) FindUserByID(_ context.Context, id int64) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, ok := r.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return user, nil
}

func (r *MemoryRepository) CreatePost(_ context.Context, params CreatePostParams) (PostWithMedia, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if params.IdempotencyKey != "" {
		if postID, ok := r.idempotencyKeys[params.IdempotencyKey]; ok {
			return r.postWithMediaLocked(postID)
		}
	}

	r.nextPostID++
	now := time.Now().UTC()
	post := Post{
		ID:        r.nextPostID,
		UserID:    params.UserID,
		Caption:   strings.TrimSpace(params.Caption),
		Status:    PostStatusDraft,
		CreatedAt: now,
	}
	r.posts[post.ID] = post
	r.stats[post.ID] = PostStats{PostID: post.ID}

	for i, item := range params.Media {
		r.nextMediaID++
		position := item.Position
		if position == 0 {
			position = i + 1
		}
		media := Media{
			ID:          r.nextMediaID,
			PostID:      post.ID,
			Position:    position,
			Type:        item.Type,
			Status:      MediaStatusUploading,
			OriginalKey: objectKey(post.ID, r.nextMediaID),
			VariantKeys: map[string]string{},
			CreatedAt:   now,
		}
		r.media[media.ID] = media
		r.mediaByPost[post.ID] = append(r.mediaByPost[post.ID], media.ID)
	}

	if params.IdempotencyKey != "" {
		r.idempotencyKeys[params.IdempotencyKey] = post.ID
	}
	return r.postWithMediaLocked(post.ID)
}

func (r *MemoryRepository) GetPost(_ context.Context, postID int64) (PostWithMedia, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.postWithMediaLocked(postID)
}

func (r *MemoryRepository) ListFeed(_ context.Context, cursor *FeedCursor, limit int) ([]PostWithMedia, *FeedCursor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ready := make([]Post, 0, len(r.posts))
	for _, post := range r.posts {
		if post.Status == PostStatusReady {
			ready = append(ready, post)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		if ready[i].CreatedAt.Equal(ready[j].CreatedAt) {
			return ready[i].ID > ready[j].ID
		}
		return ready[i].CreatedAt.After(ready[j].CreatedAt)
	})

	start := 0
	if cursor != nil {
		for i, post := range ready {
			if post.CreatedAt.UnixNano() == cursor.CreatedAtUnixNano && post.ID == cursor.ID {
				start = i + 1
				break
			}
		}
	}

	end := start + limit
	if end > len(ready) {
		end = len(ready)
	}

	items := make([]PostWithMedia, 0, end-start)
	for _, post := range ready[start:end] {
		item, err := r.postWithMediaLocked(post.ID)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}

	var next *FeedCursor
	if end < len(ready) && len(items) > 0 {
		last := items[len(items)-1].Post
		next = &FeedCursor{CreatedAtUnixNano: last.CreatedAt.UnixNano(), ID: last.ID}
	}
	return items, next, nil
}

func (r *MemoryRepository) MarkMediaProcessing(_ context.Context, mediaID int64) (Media, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.media[mediaID]
	if !ok {
		return Media{}, ErrNotFound
	}
	item.Status = MediaStatusProcessing
	item.Error = ""
	r.media[mediaID] = item
	return item, nil
}

func (r *MemoryRepository) MarkMediaReady(_ context.Context, mediaID int64, updates Media) (Media, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.media[mediaID]
	if !ok {
		return Media{}, ErrNotFound
	}
	item.Status = MediaStatusReady
	item.VariantKeys = updates.VariantKeys
	item.HLSManifestKey = updates.HLSManifestKey
	item.PosterKey = updates.PosterKey
	item.Width = updates.Width
	item.Height = updates.Height
	item.DurationMS = updates.DurationMS
	item.Error = ""
	r.media[mediaID] = item
	return item, nil
}

func (r *MemoryRepository) MarkMediaFailed(_ context.Context, mediaID int64, reason string) (Media, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.media[mediaID]
	if !ok {
		return Media{}, ErrNotFound
	}
	item.Status = MediaStatusFailed
	item.Error = reason
	r.media[mediaID] = item
	return item, nil
}

func (r *MemoryRepository) CompletePostIfReady(_ context.Context, postID int64) (Post, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	post, ok := r.posts[postID]
	if !ok {
		return Post{}, ErrNotFound
	}
	allReady := true
	anyFailed := false
	for _, mediaID := range r.mediaByPost[postID] {
		item := r.media[mediaID]
		allReady = allReady && item.Status == MediaStatusReady
		anyFailed = anyFailed || item.Status == MediaStatusFailed
	}
	switch {
	case allReady:
		post.Status = PostStatusReady
	case anyFailed:
		post.Status = PostStatusFailed
	default:
		post.Status = PostStatusDraft
	}
	r.posts[postID] = post
	return post, nil
}

func (r *MemoryRepository) LikePost(_ context.Context, userID, postID int64) (PostStats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.posts[postID]; !ok {
		return PostStats{}, ErrNotFound
	}
	if r.likes[postID] == nil {
		r.likes[postID] = map[int64]bool{}
	}
	if !r.likes[postID][userID] {
		r.likes[postID][userID] = true
		stats := r.stats[postID]
		stats.LikeCount++
		r.stats[postID] = stats
	}
	return r.stats[postID], nil
}

func (r *MemoryRepository) UnlikePost(_ context.Context, userID, postID int64) (PostStats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.posts[postID]; !ok {
		return PostStats{}, ErrNotFound
	}
	if r.likes[postID] != nil && r.likes[postID][userID] {
		delete(r.likes[postID], userID)
		stats := r.stats[postID]
		if stats.LikeCount > 0 {
			stats.LikeCount--
		}
		r.stats[postID] = stats
	}
	return r.stats[postID], nil
}

func (r *MemoryRepository) AddComment(_ context.Context, userID, postID int64, body string) (Comment, PostStats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.posts[postID]; !ok {
		return Comment{}, PostStats{}, ErrNotFound
	}
	r.nextCommentID++
	comment := Comment{
		ID:        r.nextCommentID,
		PostID:    postID,
		UserID:    userID,
		Body:      strings.TrimSpace(body),
		CreatedAt: time.Now().UTC(),
	}
	r.commentsByPost[postID] = append([]Comment{comment}, r.commentsByPost[postID]...)
	stats := r.stats[postID]
	stats.CommentCount++
	r.stats[postID] = stats
	return comment, stats, nil
}

func (r *MemoryRepository) IncrementView(_ context.Context, postID int64) (PostStats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.posts[postID]; !ok {
		return PostStats{}, ErrNotFound
	}
	stats := r.stats[postID]
	stats.ViewCount++
	r.stats[postID] = stats
	return stats, nil
}

func (r *MemoryRepository) postWithMediaLocked(postID int64) (PostWithMedia, error) {
	post, ok := r.posts[postID]
	if !ok {
		return PostWithMedia{}, ErrNotFound
	}
	items := make([]Media, 0, len(r.mediaByPost[postID]))
	for _, mediaID := range r.mediaByPost[postID] {
		items = append(items, r.media[mediaID])
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Position < items[j].Position
	})
	return PostWithMedia{
		Post:     post,
		Media:    items,
		Stats:    r.stats[postID],
		Comments: append([]Comment(nil), r.commentsByPost[postID]...),
	}, nil
}

func objectKey(postID, mediaID int64) string {
	return "originals/post-" + itoa(postID) + "/media-" + itoa(mediaID)
}

func itoa(value int64) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}
