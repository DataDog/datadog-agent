# shellcheck shell=bash
# Drop the GOPROXY entry that isn't usable from the current environment (hosts
# ending in .fabric.dog), keeping the rest. Probes the endpoint the way Go would
# (an HTTPS request with cert verification) and strips it only if that fails;
# missing curl counts as unusable.
#
# Source this file (don't execute) so the change is visible to the caller:
#   . tools/ci/sanitize-goproxy.sh
# Safe to source repeatedly; never empties GOPROXY.

__sgp_probe_url="https://depot-read-api-go.rapid-dependency-management-depot.all-clusters.local-dc.fabric.dog:8443/magicmirror/magicmirror/@current/sumdb/sum.golang.org/supported"

# Returns 0 if the endpoint completes a request over verified TLS. No --fail: we
# only care that the request completes, not its HTTP status.
__sgp_usable() {
    command -v curl >/dev/null 2>&1 \
        && curl --silent --output /dev/null --connect-timeout 3 --max-time 5 "$__sgp_probe_url"
}

case "${GOPROXY-}" in
    *.fabric.dog*)
        if ! __sgp_usable; then
            GOPROXY="$(printf '%s' "$GOPROXY" | tr '|' '\n' \
                | grep -vE '^https?://[^/]*\.fabric\.dog(:[0-9]+)?(/|$)' | paste -sd '|' -)"
            export GOPROXY
            echo "sanitize-goproxy: endpoint unusable here; stripped it from GOPROXY" >&2
        fi
        ;;
esac
