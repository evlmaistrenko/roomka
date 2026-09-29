# Roomka

A self-hosted control panel: accounts, sign-in, access control. A React UI
(`services/ui`) over a Go GraphQL server (`services/control`) with its data in
sqlite. In production one container runs both behind Caddy.

## Development

```sh
cp .env.example .env
npm install
npm run dev
```

Conventions, project layout and tests are described in
[CONTRIBUTING.md](CONTRIBUTING.md).

## Running the image

The whole app is one container, published to `evlmaistrenko/roomka` (Docker Hub)
and `ghcr.io/evlmaistrenko/roomka`. Caddy in it serves the UI, gets and renews
the TLS certificate, and proxies the API to the control server; the image fixes
its ports itself.

| Variable                               | Required | What it is                                                                                         |
| -------------------------------------- | -------- | -------------------------------------------------------------------------------------------------- |
| `ROOMKA_HOSTNAME`                      | yes      | The domain the app is served at; Caddy gets its certificate                                        |
| `ROOMKA_ACME_EMAIL`                    | yes      | Contact for the certificate authority                                                              |
| `ROOMKA_PUBLIC_URL`                    | no       | Where the UI is reached, `https://$ROOMKA_HOSTNAME` unless set. Set it only behind a further proxy |
| `ROOMKA_DISABLE_GRAPHQL_INTROSPECTION` | no       | `true` turns introspection off                                                                     |

`/data` holds the certificates and the database (`/data/roomka/roomka.db`):
mount it, or both are lost with the container.

```sh
docker run -d \
  -p 80:80 -p 443:443 -p 443:443/udp \
  -e ROOMKA_HOSTNAME=panel.example.com \
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
      ROOMKA_HOSTNAME: panel.example.com
      ROOMKA_ACME_EMAIL: you@example.com
    volumes:
      - ./data:/data
```

```sh
docker compose up -d
```

On a fresh database, the first visit to the site asks for the administrator
account. Nothing is sent by mail: a link for setting a password, for a new user
or a forgotten password, is printed to the container's log
(`docker logs <container>`), and the administrator passes it on.

## Host tuning

Caddy serves HTTP/3 over QUIC, which wants a large UDP receive buffer. Linux
caps it low by default, so Caddy logs a warning at startup (`failed to
sufficiently increase receive buffer size`) and runs with an undersized buffer. `net.core.rmem_max`/`wmem_max` aren't namespaced, so raise them on the
host (not in the container); see the [quic-go note].

```sh
sudo tee /etc/sysctl.d/99-quic-buffers.conf >/dev/null <<'EOF'
net.core.rmem_max=7500000
net.core.wmem_max=7500000
EOF
sudo sysctl --system
```

Then restart the container so Caddy requests the buffer again.

[quic-go note]: https://github.com/quic-go/quic-go/wiki/UDP-Buffer-Sizes

## Releases

Conventional commits drive [release-please]. Merging its release PR into
`master` tags a version and publishes the image to both registries.

[release-please]: https://github.com/googleapis/release-please
