#!/bin/sh
# Runs the control server and Caddy in one container: Caddy serves the UI,
# issues and renews the TLS certificate, and proxies the API to control. When
# either process exits, the container stops, so the orchestrator's restart
# policy brings both back together.
set -eu

: "${ROOMKA_HOSTNAME:?ROOMKA_HOSTNAME is required}"
: "${ROOMKA_ACME_EMAIL:?ROOMKA_ACME_EMAIL is required}"

# The UI is served from the hostname, so that is where the session cookie may
# come from and what the password links printed to the log point at. Set it
# only when the app is reached under another address, as behind a further
# proxy.
export ROOMKA_PUBLIC_URL="${ROOMKA_PUBLIC_URL:-https://${ROOMKA_HOSTNAME}}"
# The version the image was built as, the same one the UI shows.
export ROOMKA_VERSION="${ROOMKA_VERSION:-$(cat /etc/roomka/version)}"

control &
control_pid=$!
caddy run --config /etc/caddy/Caddyfile --adapter caddyfile &
caddy_pid=$!

stop() {
	kill "${control_pid}" "${caddy_pid}" 2>/dev/null || true
	wait || true
}
# docker stop sends TERM: pass it on rather than sit out the grace period and
# get killed.
trap 'stop; exit 0' TERM INT

# Sleeping in the background keeps the trap responsive: a signal interrupts
# `wait`, not a foreground `sleep`.
while kill -0 "${control_pid}" 2>/dev/null && kill -0 "${caddy_pid}" 2>/dev/null; do
	sleep 2 &
	wait $! || true
done
echo "entrypoint: a process exited, stopping the container" >&2
stop
exit 1
