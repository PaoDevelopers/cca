#!/bin/sh

set -eu

source_config=${CCA_CONFIG_PATH:-/run/cca/cca.scfgs}
runtime_config=/tmp/cca.scfgs

awk '
BEGIN { replacing = 0; depth = 0 }
!replacing && $0 ~ /^listen[[:space:]]*\{/ {
	print "listen {"
	print "\tprotocol http"
	print "\tnetwork tcp"
	print "\taddress 127.0.0.1:8193"
	print "\ttransport plain"
	print "\ttls {"
	print "\t\tcert /dev/null"
	print "\t\tkey /dev/null"
	print "\t}"
	print "}"
	replacing = 1
	depth = gsub(/\{/, "{") - gsub(/\}/, "}")
	next
}
replacing {
	depth += gsub(/\{/, "{") - gsub(/\}/, "}")
	if (depth <= 0) replacing = 0
	next
}
$0 ~ /^[[:space:]]*url[[:space:]]+/ {
	print "\turl postgresql://postgres@db:5432/cca?sslmode=disable"
	next
}
{ print }
' "$source_config" >"$runtime_config"

socat TCP-LISTEN:8192,bind=0.0.0.0,reuseaddr,fork TCP:127.0.0.1:8193 &
proxy_pid=$!

/app/cca -c "$runtime_config" &
app_pid=$!

shutdown() {
	trap - INT TERM
	kill -TERM "$app_pid" "$proxy_pid" 2>/dev/null || true
	wait "$app_pid" 2>/dev/null || true
	wait "$proxy_pid" 2>/dev/null || true
	exit 0
}

trap shutdown INT TERM

while kill -0 "$app_pid" 2>/dev/null && kill -0 "$proxy_pid" 2>/dev/null; do
	sleep 1 &
	wait $! || true
done

kill -TERM "$app_pid" "$proxy_pid" 2>/dev/null || true
wait "$app_pid" 2>/dev/null || true
wait "$proxy_pid" 2>/dev/null || true
exit 1
