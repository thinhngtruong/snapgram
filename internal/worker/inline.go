package worker

import (
	"context"

	"github.com/thinhnguyen/snapgram/internal/media"
	"github.com/thinhnguyen/snapgram/internal/store"
)

type InlineProcessor struct {
	repo  store.Repository
	media *media.Service
}

func NewInlineProcessor(repo store.Repository, media *media.Service) *InlineProcessor {
	return &InlineProcessor{repo: repo, media: media}
}

func (p *InlineProcessor) Enqueue(ctx context.Context, mediaID int64) (store.Media, error) {
	item, err := p.repo.MarkMediaProcessing(ctx, mediaID)
	if err != nil {
		return store.Media{}, err
	}

	updates := store.Media{}
	switch item.Type {
	case store.MediaTypeImage:
		updates.VariantKeys = map[string]string{
			"thumb": "variants/thumb/" + item.OriginalKey + ".webp",
			"feed":  "variants/feed/" + item.OriginalKey + ".webp",
			"full":  "variants/full/" + item.OriginalKey + ".webp",
		}
		updates.Width = 1080
		updates.Height = 1080
	case store.MediaTypeVideo:
		updates.HLSManifestKey = "hls/" + item.OriginalKey + "/master.m3u8"
		updates.PosterKey = "posters/" + item.OriginalKey + ".webp"
		updates.Width = 1920
		updates.Height = 1080
		updates.DurationMS = 60000
	default:
		return p.repo.MarkMediaFailed(ctx, mediaID, "unsupported media type")
	}

	ready, err := p.repo.MarkMediaReady(ctx, mediaID, updates)
	if err != nil {
		return store.Media{}, err
	}
	_, err = p.repo.CompletePostIfReady(ctx, ready.PostID)
	return ready, err
}
