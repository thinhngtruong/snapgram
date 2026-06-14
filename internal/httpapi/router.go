package httpapi

import (
	"log"
	"net/http"

	"github.com/thinhnguyen/snapgram/internal/auth"
	"github.com/thinhnguyen/snapgram/internal/config"
	"github.com/thinhnguyen/snapgram/internal/media"
	"github.com/thinhnguyen/snapgram/internal/posts"
	"github.com/thinhnguyen/snapgram/internal/worker"
)

type Dependencies struct {
	Config       config.Config
	Logger       *log.Logger
	Auth         *auth.Service
	Posts        *posts.Service
	Media        *media.Service
	Worker       *worker.InlineProcessor
	TokenService *auth.TokenService
}

func NewRouter(deps Dependencies) http.Handler {
	api := &API{deps: deps}
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", api.method(http.MethodGet, api.health))
	mux.HandleFunc("/v1/auth/register", api.method(http.MethodPost, api.register))
	mux.HandleFunc("/v1/auth/login", api.method(http.MethodPost, api.login))
	mux.HandleFunc("/v1/feed", api.method(http.MethodGet, api.feed))
	mux.HandleFunc("/v1/posts", api.postsRoot)
	mux.HandleFunc("/v1/posts/", api.postsNested)

	return api.recover(api.logRequests(mux))
}
