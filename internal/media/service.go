package media

import (
	"context"
	"strings"

	"github.com/thinhnguyen/snapgram/internal/store"
)

type Service struct {
	repo       store.Repository
	cdnBaseURL string
}

func NewService(repo store.Repository, cdnBaseURL string) *Service {
	return &Service{repo: repo, cdnBaseURL: strings.TrimRight(cdnBaseURL, "/")}
}

func (s *Service) CompleteUpload(ctx context.Context, mediaID int64) (store.Media, error) {
	return s.repo.MarkMediaProcessing(ctx, mediaID)
}

func (s *Service) PresignedPutURL(item store.Media) string {
	// Phase 0 uses a local placeholder. The MinIO/S3 presigner belongs behind this method.
	return "http://localhost:9000/snapgram-media/" + item.OriginalKey
}

func (s *Service) PublicURL(key string) string {
	if key == "" {
		return ""
	}
	return s.cdnBaseURL + "/" + strings.TrimLeft(key, "/")
}

func (s *Service) ReadyMediaURLs(item store.Media) map[string]string {
	urls := map[string]string{}
	for name, key := range item.VariantKeys {
		urls[name] = s.PublicURL(key)
	}
	if item.HLSManifestKey != "" {
		urls["hls"] = s.PublicURL(item.HLSManifestKey)
	}
	if item.PosterKey != "" {
		urls["poster"] = s.PublicURL(item.PosterKey)
	}
	return urls
}
