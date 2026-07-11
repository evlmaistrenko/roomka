# Roomka

Screen-share over WebTransport: a React UI (`services/ui`) talking to a Go
broadcast server (`services/broadcast`). In production a single container fronts
both with Caddy.

## Development

```sh
cp .env.example .env
npm install
npm run dev
```

## Running the image

The whole app is one container, published to `evlmaistrenko/roomka` (Docker Hub)
and `ghcr.io/evlmaistrenko/roomka`. Give it a hostname, an access-token secret,
and an ACME email — it fixes the ports itself.

```sh
docker run -d \
  -p 80:80 -p 443:443 -p 443:443/udp \
  -e ROOMKA_HOSTNAME=rooms.example.com \
  -e ROOMKA_ACCESS_SECRET=change-me-to-a-strong-random-secret \
  -e ROOMKA_ACME_EMAIL=you@example.com \
  -v "$(pwd)/data:/data" \
  evlmaistrenko/roomka:latest
```

Or with Compose:

```yaml
services:
  roomka:
    image: evlmaistrenko/roomka:latest
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"
    environment:
      ROOMKA_HOSTNAME: rooms.example.com
      ROOMKA_ACCESS_SECRET: change-me-to-a-strong-random-secret
      ROOMKA_ACME_EMAIL: you@example.com
    volumes:
      - ./data:/data
```

```sh
docker compose up -d
```

## Releases

Conventional commits drive [release-please]. Merging its release PR into
`master` tags a version and publishes the image to both registries.

[release-please]: https://github.com/googleapis/release-please
