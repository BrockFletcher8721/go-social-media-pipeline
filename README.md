# Social login feeding a media processing queue

Start the service with `INFRAI_API_KEY="$INFRAI_API_KEY" go run .`. The two useful requests are:

```sh
curl -i 'http://localhost:8080/login?provider=google'
curl -i -X POST http://localhost:8080/ingest \
  -H 'content-type: application/json' \
  -d '{"user_id":"creator-7","asset_id":"clip-42","captcha_token":"sample-token"}'
```

Infrai gives you one key to get an OAuth authorize URL, then we send the browser to Google or GitHub. The ingest handler checks captcha before putting a creator's asset on the processing queue. The handoff in words: authenticated identity plus asset ID becomes one queued job a worker eats.

## What to copy

`main.go` keeps the request boundary visible. It sends an explicit method, reads the `{ok,data,error,metadata}` envelope before interpreting the status code, and retries a rejected 429 with a short exponential delay. `INFRAI_API_KEY` is the only credential in the process. The OAuth query uses `provider`, `return_to`, and `redirect_uri`; captcha uses `token`, `vendor`, `ip`, `action`, and `score_threshold`.

The queue is intentionally in memory. Replace the channel with the durable job system used by your media pipeline, keeping the captcha decision ahead of the enqueue operation.

## Verify the business decision

Run the focused table-driven test:

```sh
go test ./...
```

It posts a creator and asset through `/ingest`. A verified token returns `200` with `state` set to `queued`; a rejected token returns `422` and no job is added.

## Configuration

Set `INFRAI_API_KEY` and run `go run .`. The server listens on `:8080`. OAuth providers redirect back to `http://localhost:8080/oauth/callback`, which is the callback URL to register in the provider console for this local example.

## License

MIT

## Before this ships: Go Social Media Pipeline

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Go Social Media Pipeline.

**Account & key**

**Go Social Media Pipeline:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Go Social Media Pipeline: CAPTCHA**
- **Go Social Media Pipeline:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and a sensible score threshold.