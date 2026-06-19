# shellcheck shell=bash
# Drop GOPROXY entries that aren't usable from the current environment, keeping
# the rest. Probes the endpoint the way Go would (an HTTPS request with cert
# verification) and strips it if that fails.
#
# Source this file (don't execute) so the change is visible to the caller:
#   . tools/ci/sanitize-goproxy.sh
# Safe to source repeatedly; never empties GOPROXY.

__sgp_host='depot-read-api-go.rapid-dependency-management-depot.all-clusters.local-dc.fabric.dog'
__sgp_probe_url="https://${__sgp_host}:8443/magicmirror/magicmirror/@current/sumdb/sum.golang.org/supported"

# Returns 0 if the endpoint completes a request over verified TLS. No --fail: we
# only care that the request completes, not its HTTP status. Missing curl counts
# as not usable.
__sgp_usable() {
    command -v curl >/dev/null 2>&1 || return 1
    curl --silent --output /dev/null --connect-timeout 3 --max-time 5 "$__sgp_probe_url"
}

# Drop entries whose host ends in .fabric.dog, keeping the '|' separators and
# every other entry.
__sgp_strip() {
    printf '%s' "${1-}" \
        | tr '|' '\n' \
        | grep -vE '^https?://[^/]*\.fabric\.dog(:[0-9]+)?(/|$)' \
        | paste -sd '|' -
}

case "${GOPROXY-}" in
    *.fabric.dog*)
        if __sgp_usable; then
            echo "sanitize-goproxy: endpoint usable here; keeping it in GOPROXY" >&2
        else
            GOPROXY="$(__sgp_strip "${GOPROXY-}")"
            export GOPROXY
            echo "sanitize-goproxy: endpoint unusable here; stripped it from GOPROXY" >&2
        fi
        ;;
esac
