#!/usr/bin/env bash

set -euo pipefail

# This script is designed to be standalone, for use in any repo, and as part of a pre-commit hook,
# so it must not have any external dependencies beyond `git`

# logs-backend will make SOURCE_REF/TARGET_REF available as part of the CI; DDCI will make DDCI_*
# variables available.  They have opposite meanings (see https://github.com/DataDog/dd-source/pull/194529#issuecomment-2775985239).
source_ref="${DDCI_PULL_REQUEST_TARGET_SHA:-${SOURCE_REF:-}}"
if [[ -z "$source_ref" ]]; then
  source_ref="$(git symbolic-ref -q refs/remotes/origin/HEAD)"
  source_ref="${source_ref##refs/remotes/}"
fi
target_ref="${DDCI_PULL_REQUEST_SOURCE_SHA:-${TARGET_REF:-HEAD}}"

# Get any dirs containing changed prebuild-devcontainer.json files, and make sure that those dirs
# are _also_ present in the list of dirs containing changed devcontainer.json files.

devcontainer_prebuild_changed_dirs=()
while IFS='' read -r line; do
  devcontainer_prebuild_changed_dirs+=("$(dirname "$line")")
done < <(git diff --name-only "$source_ref...$target_ref" -- '**/prebuild-devcontainer.json')

devcontainer_changed_dirs=()
while IFS='' read -r line; do
  devcontainer_changed_dirs+=("$(dirname "$line")")
done < <(git diff --name-only "$source_ref...$target_ref" -- '**/devcontainer.json')

bad_dirs=()
exitcode=0
for devcontainer_prebuild_dir in "${devcontainer_prebuild_changed_dirs[@]}"; do

  matched=0
  for devcontainer_dir in "${devcontainer_changed_dirs[@]}"; do
    if [[ "$devcontainer_prebuild_dir" == "$devcontainer_dir" ]]; then
      matched=1
      break
    fi
  done

  if (( !matched )); then
    bad_dirs+=("$devcontainer_prebuild_dir")
  fi

done

if (( ${#bad_dirs[@]} > 0 )); then
    printf $'\033[91m\033[1m'"ERROR:"$'\033[0m'" "
    cat <<EOF
The following prebuild-devcontainer.json files were modified but the associated devcontainer.json
files were not.  If this is a feature branch, then a campaigner PR will be opened against it with
updated devcontainer.json files; if this doesn't happen soon, please contact #workspaces in slack.
For more information, see
https://datadoghq.atlassian.net/wiki/spaces/DEVX/pages/4194009834/Creating+Specialized+Dev+Containers+and+Features#How-Can-a-Dev-Container-Launch-Faster"
EOF

    for bad_dir in "${bad_dirs[@]}"; do
      echo "  - $bad_dir/prebuild-devcontainer.json"
    done

    exitcode=1
fi

exit "$exitcode"
