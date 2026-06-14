package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/thinhnguyen/snapgram/internal/auth"
	"github.com/thinhnguyen/snapgram/internal/store"
)

type API struct {
	deps Dependencies
}

type ctxKey string

const userIDKey ctxKey = "user_id"

func (api *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (api *API) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, token, err := api.deps.Auth.Register(r.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		api.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user, "token": token})
}

func (api *API) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, token, err := api.deps.Auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		api.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "token": token})
}

func (api *API) createPost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Caption string `json:"caption"`
		Media   []struct {
			Type        string `json:"type"`
			ContentType string `json:"content_type"`
			Size        int64  `json:"size"`
		} `json:"media"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	media := make([]store.CreateMediaParams, 0, len(req.Media))
	for _, item := range req.Media {
		media = append(media, store.CreateMediaParams{
			Type:        store.MediaType(item.Type),
			ContentType: item.ContentType,
			Size:        item.Size,
		})
	}
	post, err := api.deps.Posts.Create(r.Context(), userID(r), req.Caption, r.Header.Get("Idempotency-Key"), media)
	if err != nil {
		api.writeError(w, err)
		return
	}
	uploads := make([]map[string]any, 0, len(post.Media))
	for _, item := range post.Media {
		uploads = append(uploads, map[string]any{
			"media_id":          item.ID,
			"presigned_put_url": api.deps.Media.PresignedPutURL(item),
		})
	}
	writeJSON(w, http.StatusCreated, map[string]any{"post_id": post.Post.ID, "uploads": uploads})
}

func (api *API) completeUpload(w http.ResponseWriter, r *http.Request) {
	mediaID, ok := mediaIDFromCompletePath(w, r)
	if !ok {
		return
	}
	item, err := api.deps.Worker.Enqueue(r.Context(), mediaID)
	if err != nil {
		api.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"media": item, "urls": api.deps.Media.ReadyMediaURLs(item)})
}

func (api *API) getPost(w http.ResponseWriter, r *http.Request) {
	postID, ok := postIDFromPath(w, r)
	if !ok {
		return
	}
	post, err := api.deps.Posts.Get(r.Context(), postID)
	if err != nil {
		api.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.withURLs(post))
}

func (api *API) feed(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, next, err := api.deps.Posts.Feed(r.Context(), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		api.writeError(w, err)
		return
	}
	out := make([]store.PostWithMedia, len(items))
	for i, item := range items {
		out[i] = api.withURLs(item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "next_cursor": next})
}

func (api *API) likePost(w http.ResponseWriter, r *http.Request) {
	postID, ok := postIDFromPath(w, r)
	if !ok {
		return
	}
	stats, err := api.deps.Posts.Like(r.Context(), userID(r), postID)
	if err != nil {
		api.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (api *API) unlikePost(w http.ResponseWriter, r *http.Request) {
	postID, ok := postIDFromPath(w, r)
	if !ok {
		return
	}
	stats, err := api.deps.Posts.Unlike(r.Context(), userID(r), postID)
	if err != nil {
		api.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (api *API) comment(w http.ResponseWriter, r *http.Request) {
	postID, ok := postIDFromPath(w, r)
	if !ok {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	comment, stats, err := api.deps.Posts.Comment(r.Context(), userID(r), postID, req.Body)
	if err != nil {
		api.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"comment": comment, "stats": stats})
}

func (api *API) view(w http.ResponseWriter, r *http.Request) {
	postID, ok := postIDFromPath(w, r)
	if !ok {
		return
	}
	stats, err := api.deps.Posts.View(r.Context(), postID)
	if err != nil {
		api.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (api *API) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if token == "" || token == header {
			api.writeError(w, store.ErrUnauthorized)
			return
		}
		userID, err := api.deps.TokenService.Parse(token)
		if err != nil {
			api.writeError(w, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userIDKey, userID)))
	}
}

func (api *API) method(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		next(w, r)
	}
}

func (api *API) postsRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/posts" {
		http.NotFound(w, r)
		return
	}
	api.method(http.MethodPost, api.requireAuth(api.createPost))(w, r)
}

func (api *API) postsNested(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 3 && parts[0] == "v1" && parts[1] == "posts" {
		api.method(http.MethodGet, api.getPost)(w, r)
		return
	}
	if len(parts) == 4 && parts[0] == "v1" && parts[1] == "posts" && parts[3] == "like" {
		if r.Method == http.MethodPost {
			api.requireAuth(api.likePost)(w, r)
			return
		}
		if r.Method == http.MethodDelete {
			api.requireAuth(api.unlikePost)(w, r)
			return
		}
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if len(parts) == 4 && parts[0] == "v1" && parts[1] == "posts" && parts[3] == "comments" {
		api.method(http.MethodPost, api.requireAuth(api.comment))(w, r)
		return
	}
	if len(parts) == 4 && parts[0] == "v1" && parts[1] == "posts" && parts[3] == "view" {
		api.method(http.MethodPost, api.view)(w, r)
		return
	}
	if len(parts) == 6 && parts[0] == "v1" && parts[1] == "posts" && parts[3] == "media" && parts[5] == "complete" {
		api.method(http.MethodPost, api.requireAuth(api.completeUpload))(w, r)
		return
	}
	http.NotFound(w, r)
}

func (api *API) withURLs(post store.PostWithMedia) store.PostWithMedia {
	for i := range post.Media {
		if post.Media[i].Status == store.MediaStatusReady {
			if post.Media[i].VariantKeys == nil {
				post.Media[i].VariantKeys = map[string]string{}
			}
		}
	}
	return post
}

func (api *API) writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "internal error"
	switch {
	case errors.Is(err, store.ErrUnauthorized), errors.Is(err, auth.ErrInvalidCredentials):
		status = http.StatusUnauthorized
		message = "unauthorized"
	case errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
		message = "not found"
	case errors.Is(err, store.ErrConflict):
		status = http.StatusConflict
		message = "conflict"
	default:
		if err != nil {
			message = err.Error()
		}
	}
	writeJSON(w, status, map[string]string{"error": message})
}

func userID(r *http.Request) int64 {
	id, _ := r.Context().Value(userIDKey).(int64)
	return id
}

func postIDFromPath(w http.ResponseWriter, r *http.Request) (int64, bool) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path parameter"})
		return 0, false
	}
	return parsePathInt(w, parts[2])
}

func mediaIDFromCompletePath(w http.ResponseWriter, r *http.Request) (int64, bool) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 6 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path parameter"})
		return 0, false
	}
	return parsePathInt(w, parts[4])
}

func parsePathInt(w http.ResponseWriter, raw string) (int64, bool) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path parameter"})
		return 0, false
	}
	return value, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
