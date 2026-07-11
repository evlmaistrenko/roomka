#!/bin/sh
# Runs Caddy (serves the UI, reverse-proxies the broadcast HTTP API, issues the
# TLS cert) and the WebTransport broadcast server in one container. Caddy owns
# the cert; the broadcast server reads it from Caddy's storage.
set -eu

HOSTNAME="${ROOMKA_HOSTNAME:?ROOMKA_HOSTNAME is required}"

# Inject the UI's runtime connection config (window.__ROOMKA_CONFIG__; see
# services/ui/src/lib/config.ts). The UI learns whether to pin the cert by
# querying /api/cert-hash, so only the connection endpoint is injected.
cat >/srv/config.js <<EOF
window.__ROOMKA_CONFIG__ = {
  hostname: "${HOSTNAME}",
  webTransportPort: "${ROOMKA_WEB_TRANSPORT_PORT}",
}
EOF

# Caddy issues the cert into this directory (ACME CA pinned in the Caddyfile, so
# the path is stable). We wait until both files exist, then point the broadcast
# server at them via ROOMKA_WEB_TRANSPORT_CERT.
CERT_DIR="/data/caddy/certificates/acme-v02.api.letsencrypt.org-directory/${HOSTNAME}"
CERT_FILE="${CERT_DIR}/${HOSTNAME}.crt"
CERT_KEY_FILE="${CERT_DIR}/${HOSTNAME}.key"

caddy run --config /etc/caddy/Caddyfile --adapter caddyfile &
caddy_pid=$!

echo "entrypoint: waiting for the TLS certificate for ${HOSTNAME}..."
tries=0
while [ ! -s "${CERT_FILE}" ] || [ ! -s "${CERT_KEY_FILE}" ]; do
	tries=$((tries + 1))
	if [ "${tries}" -gt 150 ]; then
		echo "entrypoint: timed out waiting for the certificate" >&2
		exit 1
	fi
	if ! kill -0 "${caddy_pid}" 2>/dev/null; then
		echo "entrypoint: caddy exited before issuing a certificate" >&2
		exit 1
	fi
	sleep 2
done

echo "entrypoint: certificate ready, starting the broadcast server"
export ROOMKA_WEB_TRANSPORT_CERT="static:${CERT_FILE};${CERT_KEY_FILE}"
broadcast &
broadcast_pid=$!

# Stop the container as soon as either process exits.
while kill -0 "${caddy_pid}" 2>/dev/null && kill -0 "${broadcast_pid}" 2>/dev/null; do
	sleep 5
done
echo "entrypoint: a process exited — shutting down" >&2
kill "${caddy_pid}" "${broadcast_pid}" 2>/dev/null || true
exit 1
