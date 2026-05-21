#!/usr/bin/env bash
# Generate periodic outbound TCP and UDP connections so that NPM and
# Network Path dynamic tests have live flows to observe.
#
# Reads target lines from /etc/datadog-agent/conn-gen-targets.txt
# Format: <proto>:<host>:<port>   (proto = tcp|udp)
# Blank lines and lines starting with # are ignored.

set -u

TARGETS_FILE="${TARGETS_FILE:-/etc/datadog-agent/conn-gen-targets.txt}"
TIMEOUT="${CONN_GEN_TIMEOUT:-5}"

if [[ ! -r "$TARGETS_FILE" ]]; then
    echo "conn-gen: targets file $TARGETS_FILE missing or unreadable" >&2
    exit 1
fi

probe_tcp() {
    local host="$1" port="$2"
    if [[ "$port" == "443" ]]; then
        if curl -sS -o /dev/null --max-time "$TIMEOUT" "https://${host}/"; then
            echo "conn-gen: tcp ${host}:${port} ok"
        else
            echo "conn-gen: tcp ${host}:${port} fail" >&2
        fi
    else
        if timeout "$TIMEOUT" bash -c "exec 3<>/dev/tcp/${host}/${port} && exec 3<&- 3>&-"; then
            echo "conn-gen: tcp ${host}:${port} ok"
        else
            echo "conn-gen: tcp ${host}:${port} fail" >&2
        fi
    fi
}

probe_udp() {
    local host="$1" port="$2"
    if [[ "$port" == "53" ]]; then
        if dig "+tries=1" "+time=${TIMEOUT}" "@${host}" A example.com >/dev/null 2>&1; then
            echo "conn-gen: udp ${host}:${port} ok"
        else
            echo "conn-gen: udp ${host}:${port} fail" >&2
        fi
    else
        echo "conn-gen: udp ${host}:${port} skipped (only DNS-style UDP probes supported)" >&2
    fi
}

while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%%#*}"
    line="${line//[$'\t\r ']/}"
    [[ -z "$line" ]] && continue

    IFS=: read -r proto host port <<<"$line"
    if [[ -z "$proto" || -z "$host" || -z "$port" ]]; then
        echo "conn-gen: malformed target '$line'" >&2
        continue
    fi

    case "$proto" in
        tcp) probe_tcp "$host" "$port" ;;
        udp) probe_udp "$host" "$port" ;;
        *)   echo "conn-gen: unknown protocol '$proto' in '$line'" >&2 ;;
    esac
done <"$TARGETS_FILE"
