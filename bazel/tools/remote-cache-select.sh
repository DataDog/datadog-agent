#!/usr/bin/env bash
# Buildbarn remote cache auto-selection for local (non-CI) developer builds.
#
# Sourced by tools/bazel. Defines _remote_cache_config, which echoes
# `--config=cache` when the remote cache should be enabled, or nothing.
#
# Policy is controlled by DD_BAZEL_REMOTE_CACHE:
#   auto (default) - enable only when the frontend is reachable and a token
#                    source is available (env token or vault CLI on the host).
#   on             - always enable; a failing credential helper aborts the build.
#   off            - never enable (disk cache stays active).
#
# CI selects its own endpoint in tools/bazel and never sources this logic.

_buildbarn_host=buildbarn-frontend-datadog-agent.us1.ddbuild.io
# Repo root: this file lives at <root>/bazel/tools/remote-cache-select.sh.
_repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." 2>/dev/null && pwd) || _repo_root=""

# Container detection, factored out so tests can override it deterministically.
_in_container() {
  [ -e /.dockerenv ] || [ -e /run/.containerenv ]
}

# True when an rc file opts out of the remote cache. The wrapper injects
# --config=cache on the command line, which would otherwise beat an rc-level
# --config=no-remote-cache (Bazel applies command-line options after rc ones).
_rc_opts_out() {
  local rc
  for rc in "${_repo_root:+$_repo_root/user.bazelrc}" "${HOME:+$HOME/.bazelrc}"; do
    [ -n "$rc" ] && [ -f "$rc" ] || continue
    # First non-blank char must not be '#' (skip comments), then an opt-out token.
    grep -Eq '^[[:space:]]*[^#[:space:]].*(--config=no-remote-cache|--remote_cache=[[:space:]]*$)' "$rc" && return 0
  done
  return 1
}

_file_age_secs() {
  local f="${1-}" mtime now
  # GNU/BusyBox stat first (-c %Y), then BSD/macOS (-f %m). Coerce any
  # non-numeric output (e.g. BusyBox printing default human output) to 0 so
  # the arithmetic below stays safe under `set -u`.
  mtime=$(stat -c %Y "$f" 2>/dev/null || stat -f %m "$f" 2>/dev/null || echo 0)
  case "$mtime" in
    '' | *[!0-9]*) mtime=0 ;;
  esac
  now=$(date +%s)
  echo $(( now - mtime ))
}

# Probe cache lives in a per-user tmp dir so it naturally resets on reboot.
_probe_file() {
  local uid d
  uid=$(id -u 2>/dev/null || echo 0)
  d="${TMPDIR:-/tmp}/datadog-agent-$uid"
  mkdir -p "$d" 2>/dev/null && chmod 700 "$d" 2>/dev/null || :
  echo "$d/remote-cache-probe"
}

# Reachability probe with asymmetric caching. Any HTTPS response (incl. gRPC's
# 415) counts as reachable; only a connection/TLS failure counts as unreachable.
# A positive result is sticky until the next tmp reset (losing access is almost
# always a global outage, which --remote_local_fallback already tolerates). A
# negative result is only cached for 60s so a VPN reconnect is picked up quickly.
_remote_cache_reachable() {
  local probe result
  probe=$(_probe_file)
  if [ -f "$probe" ]; then
    result=$(cat "$probe" 2>/dev/null || echo)
    [ "$result" = ok ] && return 0
    [ "$result" = no ] && [ "$(_file_age_secs "$probe")" -lt 60 ] && return 1
  fi
  if curl --silent --output /dev/null --connect-timeout 2 --max-time 4 "https://$_buildbarn_host/" 2>/dev/null; then
    echo ok >"$probe"
    return 0
  fi
  echo no >"$probe"
  return 1
}

_remote_cache_eligible() {
  local have_token=false
  [ -n "${BUILDBARN_ID_TOKEN-}" ] && have_token=true

  if _in_container; then
    # No interactive Vault login inside containers: require a pre-provided token.
    if [ "$have_token" != true ]; then
      >&2 echo "💡 Bazel remote cache skipped: no Buildbarn token in this container. Log in on the host, mint a token, and inject it, e.g.:"
      # shellcheck disable=SC2016
      >&2 echo '    export VAULT_ADDR=https://vault.us1.ddbuild.io'
      >&2 echo '    export BUILDBARN_ID_TOKEN="$('
      >&2 echo '      vault read -address="$VAULT_ADDR" -field=token identity/oidc/token/buildbarn \'
      >&2 echo '      || { vault login -address="$VAULT_ADDR" -method=oidc \'
      >&2 echo '           && vault read -address="$VAULT_ADDR" -field=token identity/oidc/token/buildbarn; }'
      >&2 echo '    )"'
      >&2 echo "    docker exec -e BUILDBARN_ID_TOKEN -it <container> ..."
      return 1
    fi
  elif [ "$have_token" != true ] && ! command -v vault >/dev/null 2>&1; then
    return 1
  fi
  _remote_cache_reachable
}

_remote_cache_config() {
  # An explicit cache config on the command line, or an rc-file opt-out, wins.
  case " $* " in
    *" --config=cache "* | *" --config=cache:"* | *" --config=no-remote-cache "*) return ;;
  esac
  _rc_opts_out && return
  case "${DD_BAZEL_REMOTE_CACHE:-auto}" in
    off) ;;
    on) echo --config=cache ;;
    auto) _remote_cache_eligible && echo --config=cache ;;
    *) >&2 echo "🔴 Unknown DD_BAZEL_REMOTE_CACHE='${DD_BAZEL_REMOTE_CACHE-}', expected auto|on|off" ;;
  esac
}
