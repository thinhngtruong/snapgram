# Snapgram API Smoke Test

Start the API:

```sh
make run
```

Register:

```sh
http POST :8080/v1/auth/register username=thinh email=thinh@example.com password=secret123
```

Create a post:

```sh
http POST :8080/v1/posts \
  Authorization:"Bearer $TOKEN" \
  Idempotency-Key:demo-1 \
  caption="first post" \
  media:='[{"type":"image","content_type":"image/jpeg","size":12345}]'
```

Mark upload complete:

```sh
http POST :8080/v1/posts/1/media/1/complete Authorization:"Bearer $TOKEN"
```

Read feed:

```sh
http :8080/v1/feed
```

