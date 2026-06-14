# Snapgram — Product & Tech Spec

A mini image/video sharing platform. Primary purpose: a vehicle to learn and exercise real system-design building blocks (cache, DB, CDN, blob store, async pipeline, circuit breaker, API gateway, rate limiting, observability). "Snapgram" is a placeholder name — rename freely.

---

# PART 1 — PRODUCT SPEC

## 1.1 Vision

A single-feed social app where a user uploads a **post** containing one or more **media items** (image or video, or a mix — a carousel), and others can view, like, and comment. The product is deliberately thin; the value is in the **infrastructure decisions underneath**, not feature richness.

## 1.2 Goals

- Let a user create a post with 1–N media items (images and/or videos).  
- Process media asynchronously and serve it efficiently via CDN.  
- Deliver a personalized-ish feed (chronological is fine for v1).  
- Support likes, comments, view counts.  
- **Primary goal:** force every target infra component to exist for a *real* reason.

## 1.3 Non-goals (explicit)

- No real recommendation ML. Feed is chronological / simple ranking.  
- No DMs, stories, live streaming, ads, monetization.  
- No mobile app — web client (or just an HTTP API \+ Postman/curl) is enough.  
- No multi-region. Single region; design *as if* multi-region matters, but don't build it.  
- Not building for real users — but **design for a notional scale** (below) so the decisions are honest.

## 1.4 Notional scale (design target, not real load)

Pick numbers that justify the architecture. These drive your decisions; you'll never actually hit them solo.

| Metric | Design target |
| :---- | :---- |
| Registered users | 10M |
| DAU | 1M |
| Posts/day | 500K (≈70% image, 30% video) |
| Avg media size | image 2 MB, video 30 MB |
| Read:write ratio | \~100:1 (feed/playback dominates) |
| Feed reads | \~50M/day |
| Peak QPS (reads) | \~5K |

Implications you should be able to defend: reads dominate → cache \+ CDN are load-bearing. Writes are heavy enough that uploads must be async. Video is \~6% of objects but the majority of bytes and almost all of the processing cost.

## 1.5 Personas & core user stories

**Creator**

- As a creator, I upload a post with one or more photos/videos and a caption.  
- I see processing status (uploading → processing → ready) and get notified when it's live.  
- A failed item tells me it failed; I can retry.

**Viewer**

- I scroll a feed and see ready posts; media loads fast.  
- I tap a video and it streams (adaptive quality), not download-then-play.  
- I like and comment; counts update quickly.

## 1.6 Functional requirements

1. Auth: register, login, token-based sessions (JWT).  
2. Create post: caption \+ 1–10 media items, mixed types allowed.  
3. Upload: client gets a presigned URL per media item, uploads directly to blob store.  
4. Processing: each item is validated and transformed (image variants / video HLS) asynchronously.  
5. Status: each media item exposes `uploading | processing | ready | failed`.  
6. Feed: paginated list of ready posts (cursor-based), newest first.  
7. Post detail: media URLs (CDN), caption, like count, comments.  
8. Engagement: like/unlike, comment, view count increment.  
9. Notifications (lightweight): "your post is ready" / "processing failed".

## 1.7 Non-functional requirements (targets to measure against)

| Property | Target |
| :---- | :---- |
| Feed read latency (p99) | \< 200 ms (cache hit path) |
| Media start-to-first-byte | \< 100 ms (CDN edge hit) |
| Image processing time | \< 10 s end-to-end |
| Video processing time | \< 5 min for a 60 s clip |
| Availability (read path) | 99.9% — reads survive worker/DB-write outages |
| Durability (uploaded media) | no data loss once upload acknowledged |
| Upload idempotency | duplicate submit ⇒ one logical post |

The availability line is the interesting one: **the read path must stay up even when processing is down.** That's what the circuit breaker and async design buy you.

---

# PART 2 — TECH SPEC

## 2.1 Architecture overview

                         ┌──────────────┐

        client ────────► │ API Gateway  │  auth, rate-limit, routing

                         └──────┬───────┘

            ┌───────────────────┼────────────────────┐

            ▼                   ▼                    ▼

     ┌────────────┐      ┌────────────┐       ┌────────────┐

     │  Upload    │      │  Metadata  │       │  Playback  │

     │  Service   │      │  Service   │       │  Service   │

     └─────┬──────┘      └─────┬──────┘       └─────┬──────┘

           │ presign           │ R/W                │ read

           ▼                   ▼                    ▼

     ┌──────────┐        ┌──────────┐         ┌──────────┐

     │ Blob (S3)│        │ Postgres │◄────────│  Redis   │

     └────┬─────┘        └────▲─────┘         └──────────┘

          │ upload event       │ status update

          ▼                    │

     ┌──────────┐         ┌────┴───────────┐

     │  Queue   │────────►│ Processing      │  dispatcher → image | video

     │ (fast/   │         │ Worker(s)       │  ⟲ circuit breaker \+ retries

     │  slow)   │         └────┬───────────┘

     └──────────┘              │ write variants/segments

                               ▼

                          ┌──────────┐     ┌──────┐

                          │ Blob (S3)│◄────│ CDN  │◄── viewers

                          └──────────┘     └──────┘

Services can start life as **one monolith** and be split later (see phased plan). The boundaries above are the eventual seams.

## 2.2 Components & the learning goal each one carries

| Component | Tech (suggested) | What you learn |
| :---- | :---- | :---- |
| API Gateway | Kong/Traefik, or a thin Go service | Centralized auth, routing, rate limiting, request shaping |
| Upload Service | Go | Presigned uploads, direct-to-blob, idempotency keys |
| Metadata Service | Go | Relational modeling, indexing, cursor pagination |
| Playback Service | Go | Read-optimized serving, cache-aside, signed CDN URLs |
| Blob store | MinIO (local) → S3 | Object storage semantics, keys, lifecycle, multipart |
| Queue | Redis Streams / NATS / RabbitMQ | Async decoupling, at-least-once, consumer groups, lanes |
| Worker | Go \+ ffmpeg / libvips | Async pipelines, dispatch, state machines |
| Cache | Redis | Cache-aside, TTL, stampede, counters, hot keys |
| DB | Postgres | Schema, transactions, indexes, read replicas (later) |
| CDN | CloudFront, or nginx reverse-proxy locally | Edge caching, hit ratio, cache keys, invalidation |
| Circuit breaker | gobreaker | Failure isolation, fallback, half-open recovery |
| Observability | OpenTelemetry → ClickHouse/Jaeger | Tracing the async path, metrics, RED method |

## 2.3 Data model (Postgres)

users(

  id BIGINT PK, username UNIQUE, email UNIQUE,

  password\_hash, created\_at

)

posts(

  id BIGINT PK, user\_id FK → users, caption TEXT,

  status TEXT,                 \-- draft|ready|failed (post is ready when all media ready)

  created\_at TIMESTAMPTZ,

  INDEX (user\_id, created\_at DESC)

)

media(

  id BIGINT PK, post\_id FK → posts, position SMALLINT,

  type TEXT,                   \-- image | video

  status TEXT,                 \-- uploading|processing|ready|failed

  s3\_key\_original TEXT,

  s3\_key\_variants JSONB,       \-- {thumb, feed, full}  for images

  hls\_manifest\_key TEXT NULL,  \-- for video

  poster\_key TEXT NULL,        \-- video thumbnail frame

  width INT, height INT, duration\_ms INT NULL,

  error TEXT NULL,

  created\_at TIMESTAMPTZ

)

likes(

  user\_id FK, post\_id FK, created\_at,

  PRIMARY KEY (user\_id, post\_id)        \-- idempotent like

)

comments(

  id BIGINT PK, post\_id FK, user\_id FK, body TEXT, created\_at,

  INDEX (post\_id, created\_at DESC)

)

\-- counters cached in Redis, periodically reconciled to:

post\_stats(

  post\_id PK, like\_count BIGINT, comment\_count BIGINT, view\_count BIGINT

)

idempotency\_keys(

  key TEXT PK, user\_id, post\_id NULL, created\_at  \-- dedupe post creation

)

Design note: **post and media are separate tables** so a carousel (mixed image+video) falls out for free, and each media item carries its own independent `status`.

## 2.4 Key APIs

POST /v1/auth/register | /login                    → JWT

POST /v1/posts                                      Idempotency-Key: \<uuid\>

  body: { caption, media: \[{type, content\_type, size}\] }

  → { post\_id, uploads: \[{ media\_id, presigned\_put\_url }\] }

  // client PUTs each file directly to blob via presigned URL,

  // then calls:

POST /v1/posts/{id}/media/{mid}/complete            → enqueues processing

GET  /v1/posts/{id}                                 → post \+ media (CDN urls, statuses)

GET  /v1/feed?cursor=\<opaque\>\&limit=20              → cursor-paginated ready posts

POST /v1/posts/{id}/like   | DELETE (unlike)

POST /v1/posts/{id}/comments

POST /v1/posts/{id}/view                            → increments cached counter

Notes: presigned uploads keep large media off your app servers. Cursor pagination (not offset) for the feed — encode `(created_at, id)` so paging is stable under inserts.

## 2.5 The core flow — polymorphic processing pipeline

This is the heart of the project. One ingress, one queue, a dispatcher that branches on type.

upload complete ─► enqueue {media\_id, type}

                        │

                 ┌──────┴──────┐  dispatcher routes by media.type

                 ▼             ▼

            IMAGE PATH     VIDEO PATH

            validate       validate

            strip EXIF     transcode → HLS (multi-bitrate)

            resize variants extract poster frame

            → WebP/AVIF    write segments \+ manifest

            write to S3    write to S3

                 └──────┬──────┘

                        ▼

            update media.status \= ready   (or failed \+ error)

            if all media in post ready ⇒ post.status \= ready

            invalidate/warm feed \+ post cache; emit notification

**Two queue lanes.** Images finish in seconds; a 4-minute transcode in the same lane would starve them. Use a `fast` lane (image/resize) and a `slow` lane (video/transcode), or priority consumer groups. Decide this early — it's NFR-driven, not premature.

**Status state machine** (per media item):

uploading ──complete──► processing ──ok──► ready

                            │

                            └──error/timeout──► failed ──retry──► processing

## 2.6 Reliability scaffolding (same for both paths)

- **Idempotency.** `Idempotency-Key` on post creation (dedupe submits). Queue is at-least-once, so the worker must be idempotent: keying on `media_id` \+ a processing version means reprocessing the same item is safe (overwrite same S3 keys, same DB row).  
- **Circuit breaker** (gobreaker) wraps the external processor call (ffmpeg service / libvips / any out-of-process dep). On repeated failure it opens, the worker fast-fails that dependency and the item goes to a retry/backoff queue instead of hammering a sick service. Half-open probes recovery. *Crucially, the read/feed path doesn't depend on this — it keeps serving already-ready posts while processing is degraded.*  
- **Retries with backoff \+ dead-letter.** N attempts with exponential backoff; exhausted ⇒ DLQ \+ `status=failed` \+ user-visible error.  
- **Timeouts** on every external call. A transcode that hangs must be killed, not waited on forever.

## 2.7 Caching strategy (Redis)

| Data | Pattern | Notes |
| :---- | :---- | :---- |
| Post metadata | cache-aside, TTL \~5 min | warm on read; invalidate on edit |
| Feed page (per user or global) | cache-aside, short TTL | watch stampede — use single-flight / jittered TTL |
| Like/view/comment counts | Redis as source-of-truth counter | `INCR`; flush to `post_stats` in batches |
| Hot post protection | request coalescing | one DB read fills cache for many concurrent misses |

Counters are the fun part: writing every view straight to Postgres won't scale, so increment in Redis and reconcile periodically — at-least-once semantics mean you accept small drift or use a more careful flush.

## 2.8 Media serving & CDN

- Images: serve variant objects (`thumb/feed/full`) directly from CDN-fronted blob.  
- Video: serve the `.m3u8` manifest \+ `.ts`/`fMP4` segments through the CDN. Player does adaptive bitrate selection — segments are individually cacheable at the edge, which is the entire reason HLS \+ CDN compose so well.  
- Use **signed CDN URLs** with expiry for private/authorized media (learn cache-key vs signature interaction).  
- Locally, simulate the CDN with **nginx as a caching reverse proxy** in front of MinIO — you get real `X-Cache: HIT/MISS` headers to measure hit ratio without paying for CloudFront.

## 2.9 API gateway & rate limiting

The gateway owns cross-cutting concerns so services stay clean:

- JWT validation \+ identity propagation (inject `user_id` header downstream).  
- Routing to upload/metadata/playback.  
- **Rate limiting** — per-user and per-IP. This is where your distributed rate-limiter work plugs in directly (token bucket in Redis, `SET NX EX` / Lua for atomicity). Tighter limits on the expensive upload endpoint than on reads.

## 2.10 Observability

- **Tracing** (OpenTelemetry): one trace\_id from upload → queue → worker → status update. Tracing the *async hop* (propagating context through the queue message) is the lesson — it's harder and more valuable than tracing a sync request.  
- **Metrics** (RED): rate/errors/duration per endpoint and per worker stage; queue depth per lane; cache hit ratio; circuit-breaker state.  
- **Export** to ClickHouse (or Jaeger/Prometheus \+ Grafana) — ties into the OTel \+ ClickHouse architecture you've already looked at.

## 2.11 Suggested tech stack

| Concern | Choice |
| :---- | :---- |
| Language | Go |
| HTTP | chi / gin / net/http |
| DB | Postgres (sqlc or pgx, no heavy ORM) |
| Cache | Redis |
| Blob | MinIO local → S3 prod |
| Queue | Redis Streams (simplest) or NATS/RabbitMQ |
| Image proc | libvips (govips) — fast, low memory |
| Video proc | ffmpeg (exec or a wrapper service) |
| Breaker | sony/gobreaker |
| Gateway | Traefik/Kong, or a thin Go gateway you write |
| CDN (local) | nginx caching reverse proxy |
| Tracing | OpenTelemetry-Go → Jaeger or ClickHouse |
| Orchestration | docker-compose (k8s only if you want that lesson) |

## 2.12 Phased delivery plan

| Phase | You build | Components unlocked | Lesson |
| :---- | :---- | :---- | :---- |
| **0** | Monolith: auth, post CRUD, sync upload to MinIO, raw playback | DB, Blob | Get the data model right first |
| **1** | Cache-aside for posts/feed; Redis counters for likes/views | Cache | TTL, stampede, counter flush |
| **2** | Presigned uploads \+ queue \+ worker; **image path** (resize/variants/EXIF) | Queue, Worker | Async pipeline, status state machine |
| **3** | **Video path**: ffmpeg → HLS, poster frame, fast/slow lanes | (extends worker) | Dispatch, lane isolation |
| **4** | Circuit breaker \+ retries \+ DLQ \+ idempotent workers | Circuit breaker | Failure isolation, read path stays up |
| **5** | CDN in front of blob; measure hit ratio; signed URLs | CDN | Edge caching, cache keys |
| **6** | Split monolith into upload/metadata/playback behind gateway; rate limiting | API Gateway | Service boundaries, cross-cutting concerns |
| **7** (stretch) | OTel tracing through the async path; metrics dashboards; read replica | Observability | Trace the queue hop, RED metrics |

Ship Phase 0 end-to-end before touching Phase 1 — a thin vertical slice beats a wide unfinished base.

## 2.13 Key decisions to be able to defend (interview-relevant)

- Why **async upload** instead of processing inline? (latency, failure isolation, scaling workers independently)  
- Why **presigned direct-to-blob** instead of proxying through the app? (offload bandwidth/memory)  
- Why **separate post/media tables**? (carousels, per-item status)  
- Why **Redis counters** instead of `UPDATE ... SET count = count+1`? (write hotspot, lock contention)  
- Why **cursor pagination** over offset? (stability under inserts, performance at depth)  
- Where does the **circuit breaker** sit and what's the **fallback**? (around the processor; fallback \= enqueue retry, keep read path alive)  
- How does the system stay **read-available** while processing is down? (the whole point — async \+ cache \+ already-ready content)

