#!/usr/bin/env bash
# Sourced by tools/bazel for local builds. DD_BAZEL_ADMS=auto|on|off controls
# injection of --config=adms independently of Buildbarn remote cache selection.

_adms_repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." 2>/dev/null && pwd) || _adms_repo_root=""

_adms_options_configured() {
  local arg next_config=false
  for arg in "$@"; do
    case "$arg" in
      --) break ;;
      --config=adms | --config=dd-internal | --downloader_config | --downloader_config=*) return 0 ;;
      adms | dd-internal) [ "$next_config" = true ] && return 0 ;;
    esac
    next_config=false
    [ "$arg" = --config ] && next_config=true
  done
  return 1
}

# A user-supplied downloader config (including an empty opt-out) must not be
# overwritten by command-line injection. Also avoid expanding ADMS twice.
_adms_rc_configured() {
  local rc line
  local -a options
  for rc in "${_adms_repo_root:+$_adms_repo_root/user.bazelrc}" "${HOME:+$HOME/.bazelrc}"; do
    [ -n "$rc" ] && [ -f "$rc" ] || continue
    while IFS= read -r line || [ -n "$line" ]; do
      line=${line%%#*}
      read -r -a options <<< "$line"
      case "${options[0]-}" in
        '' | *:*) continue ;; # Comments/blanks and inactive named configs.
      esac
      _adms_options_configured "${options[@]:1}" && return 0
    done < "$rc"
  done
  return 1
}

_adms_file_age_secs() {
  local mtime now
  # GNU/BusyBox stat first, then BSD/macOS. Guard non-numeric output under -u.
  mtime=$(stat -c %Y "$1" 2>/dev/null || stat -f %m "$1" 2>/dev/null || echo 0)
  case "$mtime" in
    '' | *[!0-9]*) mtime=0 ;;
  esac
  now=$(date +%s)
  echo $(( now - mtime ))
}

_adms_reachable() {
  local uid dir probe result
  uid=$(id -u 2>/dev/null || echo 0)
  dir="${TMPDIR:-/tmp}/datadog-agent-$uid"
  probe="$dir/adms-probe"
  # Cache both outcomes for 60s. Unlike the remote cache, the downloader cannot
  # fall back locally if network access is lost, so success must also expire.
  if [ -f "$probe" ] && [ "$(_adms_file_age_secs "$probe")" -lt 60 ]; then
    result=$(cat "$probe" 2>/dev/null || :)
    [ "$result" = ok ] && return 0
    [ "$result" = no ] && return 1
  fi
  mkdir -p "$dir" 2>/dev/null && chmod 700 "$dir" 2>/dev/null || :
  # The host root returns HTTP 400. Probe a small registry resource instead;
  # --fail rejects HTTP errors (including proxy 502s), not just TLS failures.
  if curl --silent --fail --output /dev/null --connect-timeout 2 --max-time 4 \
      https://depot-read-api-bzl.us1.ddbuild.io/bcr.bazel.build/bazel_registry.json 2>/dev/null; then
    { echo ok >"$probe"; } 2>/dev/null || :
    return 0
  fi
  { echo no >"$probe"; } 2>/dev/null || :
  return 1
}

_adms_config() {
  _adms_options_configured "$@" && return 0
  _adms_rc_configured && return 0
  case "${DD_BAZEL_ADMS:-auto}" in
    off) ;;
    on) echo --config=adms ;;
    auto) _adms_reachable && echo --config=adms ;;
    *) >&2 echo "🔴 Unknown DD_BAZEL_ADMS='${DD_BAZEL_ADMS-}', expected auto|on|off" ;;
  esac
}
