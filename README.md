# Snapgram

Mini image/video sharing platform scaffolded from [SPEC.md](SPEC.md).

The current codebase is a Phase 0 monolith skeleton:

- Go HTTP API with auth, post creation, upload completion, feed, likes, comments, and views.
- In-memory repository so the service runs without downloading database drivers.
- Postgres schema migration matching the spec.
- Docker Compose for Postgres, Redis, MinIO, local nginx CDN cache, and Jaeger.
- Package boundaries for future extraction into upload, metadata, playback, and worker services.

## Quick Start

```sh
cp .env.example .env
make run
```

Health check:

```sh
curl http://localhost:8080/healthz
```

Local infrastructure:

```sh
make docker-up
```

See [docs/api/httpie.md](docs/api/httpie.md) for a small API smoke test.

## Project Layout

```text
cmd/api              API entrypoint
internal/auth        Registration, login, token issuing
internal/httpapi     HTTP routing, handlers, middleware
internal/media       Upload URL and CDN URL helpers
internal/posts       Post, feed, engagement use cases
internal/store       Domain models and repository interface
internal/worker      Inline placeholder for the async processor
migrations           Postgres schema
deployments/nginx    Local CDN cache simulation
```

## Next Build Steps

1. Replace `internal/store.MemoryRepository` with a Postgres implementation using `pgx` or `sqlc`.
2. Replace placeholder upload URLs with MinIO/S3 presigned PUT URLs.
3. Move `worker.InlineProcessor` to Redis Streams lanes: `media:fast` for images and `media:slow` for video.
4. Add Redis cache-aside for feed/post reads and Redis counters for likes/views/comments.
5. Add real image/video processors, retries, DLQ, circuit breaker, and OpenTelemetry traces.
