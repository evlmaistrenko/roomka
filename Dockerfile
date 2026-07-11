# syntax=docker/dockerfile:1

# --- build the UI static bundle ---
FROM node:22-alpine AS ui
WORKDIR /app
COPY package.json package-lock.json ./
COPY services/ui/package.json services/ui/package.json
# HUSKY=0 skips the `prepare` git-hook install (no .git in the build context).
RUN HUSKY=0 npm ci
COPY services/ui services/ui
RUN npm run build --workspace services/ui

# --- build the broadcast binary (static, no cgo) ---
FROM golang:1.26-alpine AS broadcast
WORKDIR /src
COPY services/broadcast .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /broadcast .

# --- runtime: Caddy serves the UI and issues the TLS cert; the broadcast server reads it ---
FROM caddy:2-alpine
COPY --from=broadcast /broadcast /usr/local/bin/broadcast
COPY --from=ui /app/services/ui/dist /srv
COPY Caddyfile /etc/caddy/Caddyfile
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh
# Ports the image fixes: WebTransport on 443/udp (Caddy has HTTP/3 disabled so it
# owns that port — see the Caddyfile) and the broadcast HTTP API on an internal
# port Caddy reverse-proxies (not exposed). The runtime supplies the rest
# (ROOMKA_HOSTNAME, ROOMKA_ACCESS_SECRET, ROOMKA_ACME_EMAIL).
ENV ROOMKA_WEB_TRANSPORT_PORT=443 \
    ROOMKA_API_PORT=8080
# 80 = ACME HTTP challenge + redirect; 443/tcp = UI (Caddy, h1/h2);
# 443/udp = the broadcast server's WebTransport (Caddy's HTTP/3 disabled for it).
EXPOSE 80 443 443/udp
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
