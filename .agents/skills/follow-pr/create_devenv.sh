#!/bin/sh

set -e pipefail

# Get the repo path
REPO=$(git rev-parse --show-toplevel)

# Create a unique ID
ID=babysit-pr-attach-$(uuidgen | cut -d'-' -f1)

# Use --no-pull to make sure it starts quickly
# Explicitly pass the repo to avoid cloning or missing the automatic bind-mount. Note it will be mounted as `/repos/the/full/absolute/path` inside the container
# But the CWD is changed appropriately when doing this way, so it's fine
dda env dev start --no-pull --repo "${REPO}" --id "${ID}"

echo "Created env: ${ID}"
