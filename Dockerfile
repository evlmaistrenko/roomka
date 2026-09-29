# syntax=docker/dockerfile:1

# --- the UI: a static bundle ---
FROM node:22-alpine AS ui
WORKDIR /app
COPY package.json package-lock.json ./
COPY services/ui/package.json services/ui/package.json
# HUSKY=0 skips the `prepare` git-hook install (no .git in the build context).
RUN HUSKY=0 npm ci
COPY services/ui services/ui
RUN npm run build --workspace services/ui
# The release version lives in the root package.json, where release-please
# bumps it. The UI reads it at build time; the server is handed it at start.
RUN node -p "require('./package.json').version" > /version

# --- the control server: one static binary, no cgo (the sqlite driver is pure Go) ---
FROM golang:1.26-alpine AS control
WORKDIR /src
COPY services/control/go.mod services/control/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY services/control .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /control .

# --- runtime: Caddy serves the UI, terminates TLS and proxies to control ---
FROM caddy:2-alpine
COPY --from=control /control /usr/local/bin/control
COPY --from=ui /app/services/ui/dist /srv
COPY --from=ui /version /etc/roomka/version
COPY Caddyfile /etc/caddy/Caddyfile
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh
# What the image fixes itself. /data is the Caddy image's volume, holding the
# certificates already; the database goes next to them so one volume keeps
# everything. The runtime supplies the rest, ROOMKA_HOSTNAME and
# ROOMKA_ACME_EMAIL (see the README).
ENV ROOMKA_API_PORT=8080 \
    ROOMKA_DATABASE_PATH=/data/roomka/roomka.db
# 80: the ACME HTTP challenge and the redirect to https. 443/tcp: HTTP/1.1 and
# HTTP/2. 443/udp: HTTP/3. The control server's port stays inside.
EXPOSE 80 443 443/udp
# Asks control directly, past Caddy: /health is not published.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s \
    CMD wget -q -O /dev/null "http://127.0.0.1:${ROOMKA_API_PORT}/health" || exit 1
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
