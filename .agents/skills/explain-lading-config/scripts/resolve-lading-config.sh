#!/usr/bin/env bash
# Resolve the target lading.yaml for the explain-lading-config skill.
#
# Scope: experiments under `test/regression/cases/` and
# `test/regression/x-disabled-cases/`.
# The skill deliberately does not enumerate `ebpf/cases/` (split-mode)
# or `ebpf/config-only/cases/` yet. They have different semantics.
#
# Usage:
#   resolve-lading-config.sh [ARG]
#
# When ARG is omitted, emits one `<experiment>\t<path>` line per
# discovered lading.yaml (tab-separated). Callers list these to the user.
#
# When ARG is provided, resolves it to exactly one path and prints that path.
# Exits non-zero with candidate paths on stderr if the argument is ambiguous,
# or with "not found" if nothing matches.
#
# Accepted ARG forms:
#   - absolute or relative path to a lading.yaml
#   - path containing '/' — treated as a file path
#   - substring of an experiment name (plain substring, no shell glob
#     characters required)
#   - glob with '*' or '?' — matched against experiment names, not paths
#
# Experiment name = the case directory name, i.e. the parent of `lading/`
# in `test/regression/cases/<case>/lading/lading.yaml`.

set -euo pipefail

repo_root() {
    git rev-parse --show-toplevel 2>/dev/null || pwd
}

# Exit early with a clear error if we cannot locate the regression suite.
# Otherwise a user running this from the wrong directory (e.g. /tmp) would
# see a silent "no matches" for every query.
require_regression_dir() {
    local root
    root="$(repo_root)"
    if [[ ! -d "$root/test/regression/cases" ]]; then
        cat >&2 <<EOF
no test/regression/cases/ directory under $root

This script must run from inside the DataDog/datadog-agent repository.
\`cd\` into the repo (or a subdirectory of it) and re-run.
EOF
        exit 4
    fi
}

# Emit NUL-delimited paths for all lading.yaml files under
# test/regression/cases (active) and test/regression/x-disabled-cases.
find_configs() {
    local root
    root="$(repo_root)"
    local d
    for d in cases x-disabled-cases; do
        [[ -d "$root/test/regression/$d" ]] || continue
        find "$root/test/regression/$d" -type f -name lading.yaml -print0
    done
}

# Extract the display name for a lading.yaml path:
# .../{cases,x-disabled-cases}/<case>/lading/lading.yaml -> <case>
display_name() {
    local path="$1"
    basename "$(dirname "$(dirname "$path")")"
}

list_all() {
    local path
    while IFS= read -r -d '' path; do
        printf '%s\t%s\n' "$(display_name "$path")" "$path"
    done < <(find_configs) | sort
}

# Render `<name>\t<path>` rows with a trailing `(disabled)` column for
# rows that live under `x-disabled-cases/`. The first two fields stay
# tab-separated so existing parsers still work.
annotate_for_display() {
    local name path
    while IFS=$'\t' read -r name path; do
        [[ -z "$name" ]] && continue
        if [[ "$path" == */x-disabled-cases/* ]]; then
            printf '%s\t%s\t%s\n' "$name" "$path" "(disabled)"
        else
            printf '%s\t%s\n' "$name" "$path"
        fi
    done
}

# Emit up to three "did you mean?" suggestions on stderr.
# Scoring: count of tokens from the query (split on `_`, space, or `-`) that
# appear as substrings of the candidate name. Matching is case-insensitive
# because all experiment names are lowercase. Ties broken by shorter name.
suggest_near_matches() {
    local query="$1" all="$2"
    local lower_query
    lower_query="$(printf '%s' "$query" | tr '[:upper:]' '[:lower:]')"
    local IFS_=$IFS
    # shellcheck disable=SC2206
    IFS=$' \t_-' read -ra tokens <<< "$lower_query"
    IFS=$IFS_
    local scored="" name path score token
    while IFS=$'\t' read -r name path; do
        [[ -z "$name" ]] && continue
        score=0
        for token in "${tokens[@]}"; do
            [[ -n "$token" && "$name" == *"$token"* ]] && score=$((score + 1))
        done
        if [[ "$score" -gt 0 ]]; then
            scored+="$score"$'\t'"$name"$'\n'
        fi
    done <<< "$all"
    if [[ -n "$scored" ]]; then
        echo "did you mean?" >&2
        printf '%s' "$scored" | sort -k1,1rn -k2,2 | head -3 | cut -f2 | sed 's/^/  /' >&2
    fi
}

resolve_one() {
    local arg="$1"

    # 1) Direct path — resolve and return if the file exists AND looks like a
    #    lading config. We reject arbitrary existing files (e.g. /etc/hosts)
    #    to avoid the downstream explainer operating on something unrelated.
    if [[ "$arg" == */* || "$arg" == *.yaml ]]; then
        # Expand a leading `~/` by hand — [[ -f ]] does not tilde-expand a
        # quoted literal, and `${arg#~/}` has subtle tilde-expansion behaviour
        # in the pattern, so just slice off the first two characters.
        case "$arg" in
            "~/"*) arg="$HOME/${arg:2}" ;;
            "~")   arg="$HOME" ;;
        esac
        if [[ ! -f "$arg" ]]; then
            echo "not found: $arg" >&2
            return 2
        fi
        if [[ "$(basename "$arg")" != "lading.yaml" ]] \
           && ! head -200 "$arg" 2>/dev/null | grep -qE '^(generator|blackhole|target_metrics)\s*:'; then
            echo "not a lading config: $arg" >&2
            echo "expected a file named 'lading.yaml' or one containing a top-level" >&2
            echo "'generator:', 'blackhole:', or 'target_metrics:' key" >&2
            return 2
        fi
        printf '%s\n' "$(cd "$(dirname "$arg")" && pwd)/$(basename "$arg")"
        return 0
    fi

    # 2) Match against experiment names.
    #
    # Matching precedence:
    #   a. Exact name match wins outright (even if the arg is a substring of
    #      other names — e.g. `uds_dogstatsd_to_api` should not be ambiguous
    #      just because `uds_dogstatsd_to_api_v3` exists).
    #   b. Else if the arg contains glob metachars, shell-glob against names.
    #   c. Else plain substring match (so `security_mean` matches
    #      `quality_gate_security_mean_fs_load`).
    # Matching is case-insensitive — all experiment names are lowercase in
    # the repo, so treating the arg as lowercase adds ergonomics (typing
    # `QUALITY_GATE_IDLE` still works) without ambiguity.
    local all exact matches path name lower_arg lower_name
    all="$(list_all)"
    exact=""
    matches=""
    lower_arg="$(printf '%s' "$arg" | tr '[:upper:]' '[:lower:]')"
    while IFS=$'\t' read -r name path; do
        lower_name="$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')"
        if [[ "$lower_name" == "$lower_arg" ]]; then
            exact="$name"$'\t'"$path"$'\n'
        fi
        if [[ "$lower_arg" == *[*?[]* ]]; then
            # shellcheck disable=SC2053
            [[ "$lower_name" == $lower_arg ]] && matches="$matches$name"$'\t'"$path"$'\n'
        else
            [[ "$lower_name" == *"$lower_arg"* ]] && matches="$matches$name"$'\t'"$path"$'\n'
        fi
    done <<< "$all"

    if [[ -n "$exact" ]]; then
        printf '%s' "$exact" | cut -f2
        return 0
    fi

    local count
    count="$(printf '%s' "$matches" | grep -c . || true)"
    if [[ "$count" -eq 0 ]]; then
        echo "no lading.yaml matches '$arg'" >&2
        suggest_near_matches "$arg" "$all" >&2
        return 2
    fi
    if [[ "$count" -gt 1 ]]; then
        echo "multiple matches for '$arg':" >&2
        printf '%s' "$matches" | annotate_for_display >&2
        return 3
    fi
    printf '%s' "$matches" | cut -f2
}

require_regression_dir

if [[ $# -eq 0 || -z "${1-}" ]]; then
    list_all | annotate_for_display
else
    resolve_one "$1"
fi
