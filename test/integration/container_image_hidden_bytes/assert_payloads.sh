#!/usr/bin/env bash
set -euo pipefail

# Run from the repository root. This reads local images and fakeintake only.
qa_url=${1:-http://127.0.0.1:18080}
qa_mode=${2:-present}
case "$qa_mode" in present|absent) ;; *) echo "Usage: bash $0 [fakeintake-url] [present|absent] [scenario ...]" >&2; exit 2 ;; esac
qa_cli=test/fakeintake/build/fakeintakectl
test -x "$qa_cli"
qa_cases=(seeded deleted replaced combined replaced-deleted same-step
  hardlink-seeded hardlink-one-deleted hardlink-both-deleted hardlink-replaced hardlink-old-deleted hardlink-all-deleted)
if (( $# > 2 )); then qa_cases=("${@:3}"); fi

for qa_case in "${qa_cases[@]}"; do
  case "$qa_case" in
    seeded|deleted|replaced|combined|replaced-deleted|same-step)
      qa_seed_name=seeded; qa_replacement_name=replaced ;;
    hardlink-seeded|hardlink-one-deleted|hardlink-both-deleted|hardlink-replaced|hardlink-old-deleted|hardlink-all-deleted|native-only)
      qa_seed_name=hardlink-seeded; qa_replacement_name=hardlink-replaced ;;
    *) echo "Unknown scenario: $qa_case" >&2; exit 2 ;;
  esac
  qa_seed=$(docker image inspect "localhost/hidden-bytes/$qa_seed_name:qa" | jq -er '.[0].RootFS.Layers[-1]')
  qa_replacement=$(docker image inspect "localhost/hidden-bytes/$qa_replacement_name:qa" | jq -er '.[0].RootFS.Layers[-1]')
  qa_name="localhost/hidden-bytes/$qa_case"
  qa_inspect=$(docker image inspect "$qa_name:qa")
  # With Docker's containerd store, inspect .Id can be an index digest.
  qa_id=$(docker image save "$qa_name:qa" | tar -xOf - manifest.json | jq -er '
    if length != 1 then error("expected one platform image") else
      .[0].Config | if startswith("blobs/sha256/") then
        "sha256:" + ltrimstr("blobs/sha256/")
      elif endswith(".json") then "sha256:" + rtrimstr(".json")
      else error("unsupported config path") end
    end')
  qa_expected=$(jq -ce --arg scenario "$qa_case" --arg seed "$qa_seed" --arg replacement "$qa_replacement" '
    [.[0].RootFS.Layers[] | . as $id | {
      digest: $id,
      hidden_bytes: (if $id == $seed then
        if $scenario == "deleted" then 8192
        elif $scenario == "replaced" or $scenario == "replaced-deleted" then 4096
        elif $scenario == "combined" then 12288
        elif $scenario == "hardlink-both-deleted" or $scenario == "hardlink-old-deleted" or
             $scenario == "hardlink-all-deleted" or $scenario == "native-only" then 4096
        else 0 end
      elif $id == $replacement and ($scenario == "replaced-deleted" or
           $scenario == "hardlink-all-deleted" or $scenario == "native-only") then 12288
      else 0 end)
    }]' <<<"$qa_inspect")
  qa_pass=false
  for ((qa_attempt = 1; qa_attempt <= 60; qa_attempt++)); do
    if qa_payloads=$("$qa_cli" --url "$qa_url" filter container-images --name "$qa_name") &&
      jq -e --arg image_id "$qa_id" --arg mode "$qa_mode" --argjson expected "$qa_expected" '
        any(.[];
          .digest == $image_id and
          ([.layers[] | select((.digest // "") != "")] as $layers |
            ($layers | map(.digest)) == ($expected | map(.digest)) and
            if $mode == "present" then
              all($layers[]; has("hidden_bytes")) and
              ($layers | map({digest, hidden_bytes})) == $expected
            else all($layers[]; has("hidden_bytes") | not) end)
        )' <<<"$qa_payloads" >/dev/null; then
      qa_pass=true
      break
    fi
    sleep 2
  done
  if [[ "$qa_pass" != true ]]; then
    echo "FAIL $qa_case ($qa_id): expected $qa_mode values $qa_expected" >&2
    echo "${qa_payloads:-No payload received}" >&2
    exit 1
  fi
  if [[ "$qa_mode" == present ]]; then
    echo "PASS $qa_case ($qa_id): $qa_expected"
  else
    echo "PASS $qa_case ($qa_id): hidden_bytes absent on every filesystem layer"
  fi
done
