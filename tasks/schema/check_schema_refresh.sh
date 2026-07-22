#!/usr/bin/env bash
# Verify that the committed Agent config schema has been refreshed.
#
# Usage: check_schema_refresh.sh <project_dir>
#
# Assumes the Agent has already been built and `dda inv -- schema.generate` has
# already been run, regenerating the YAML schema files in place under
# pkg/config/schema/yaml/. Fails if regeneration produced any change (modified
# or new files) to the committed schema, which means the author forgot to
# refresh the schema after changing the configuration.

set -euo pipefail

PROJECT_DIR="$1"
SCHEMA_DIR="$PROJECT_DIR/pkg/config/schema/yaml"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MESSAGE_FILE="$SCRIPT_DIR/schema_refresh_failure_message.txt"

# --porcelain reports both tracked-file modifications and brand-new (untracked)
# schema files, so a newly split section that was never committed is caught too.
CHANGES=$(git -C "$PROJECT_DIR" status --porcelain -- "$SCHEMA_DIR")

if [ -n "$CHANGES" ]; then
  echo "ERROR: The generated Agent config schema is out of date. Changed files:"
  echo "$CHANGES"
  echo "---"
  # Show the actual diff for tracked files to make the required update obvious.
  git -C "$PROJECT_DIR" diff -- "$SCHEMA_DIR" || true
  echo ""
  cat "$MESSAGE_FILE"
  exit 1
fi

echo "[Success] The committed Agent config schema is up to date."
