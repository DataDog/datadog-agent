#!/usr/bin/env bash
# Validate that schema-based template generation matches legacy Go template generation.
#
# Usage: check_config_templates.sh <project_dir>
#
# Generates all config templates two ways (new schema-based, old Go template) and
# asserts both produce identical output (ignoring trailing whitespace).

set -euo pipefail

PROJECT_DIR="$1"
CORE_SCHEMA="$PROJECT_DIR/pkg/config/schema/core_schema.yaml"
SYSPROBE_SCHEMA="$PROJECT_DIR/pkg/config/schema/system-probe_schema.yaml"
RENDER_CONFIG="$PROJECT_DIR/pkg/config/render_config/render_config.go"
CONFIG_DIR="$PROJECT_DIR/pkg/config"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MESSAGE_FILE="$SCRIPT_DIR/template_diff_failure_message.txt"

NEW_TEMPLATES=$(mktemp -d)
OLD_TEMPLATES=$(mktemp -d)
trap 'rm -rf "$NEW_TEMPLATES" "$OLD_TEMPLATES"' EXIT

dda inv -- schema.template-all "$CORE_SCHEMA" "$SYSPROBE_SCHEMA" "$NEW_TEMPLATES"
go run "$RENDER_CONFIG" "$OLD_TEMPLATES" "$CONFIG_DIR"

FAILED=0

NEW_COUNT=$(find "$NEW_TEMPLATES" -maxdepth 1 -name "*.yaml" | wc -l)
OLD_COUNT=$(find "$OLD_TEMPLATES" -maxdepth 1 -name "*.yaml" | wc -l)
if [ "$NEW_COUNT" != "$OLD_COUNT" ]; then
  echo "ERROR: Different number of generated files: new=$NEW_COUNT, old=$OLD_COUNT"
  echo "New files: $(ls "$NEW_TEMPLATES"/)"
  echo "Old files: $(ls "$OLD_TEMPLATES"/)"
  FAILED=1
fi

NEW_FILES=$(find "$NEW_TEMPLATES" -maxdepth 1 -name "*.yaml" -exec basename {} \; | sort)
OLD_FILES=$(find "$OLD_TEMPLATES" -maxdepth 1 -name "*.yaml" -exec basename {} \; | sort)
if [ "$NEW_FILES" != "$OLD_FILES" ]; then
  echo "ERROR: File names don't match between new and old templates"
  echo "New files: $NEW_FILES"
  echo "Old files: $OLD_FILES"
  FAILED=1
fi

# Strip trailing whitespace before diffing instead of using diff's
# --ignore-trailing-space, which is a GNU extension not supported on macOS.
# Use awk instead of sed: awk always appends a newline after print, so the
# output consistently ends with a newline regardless of whether the input does.
# macOS sed does not add a trailing newline for files that lack one, which
# causes spurious "No newline at end of file" diffs.
strip_trailing() {
  awk '{ sub(/[[:space:]]+$/, ""); print }' "$1"
}

for file in $NEW_FILES; do
  if [ ! -f "$OLD_TEMPLATES/$file" ]; then
    continue
  fi
  if ! diff <(strip_trailing "$NEW_TEMPLATES/$file") <(strip_trailing "$OLD_TEMPLATES/$file") > /dev/null 2>&1; then
    echo "ERROR: $file differs between new (schema-based) and old (template-based) generation:"
    diff <(strip_trailing "$NEW_TEMPLATES/$file") <(strip_trailing "$OLD_TEMPLATES/$file") || true
    echo "---"
    FAILED=1
  fi
done

if [ "$FAILED" -ne 0 ]; then
  echo ""
  cat "$MESSAGE_FILE"
  exit 1
else
  echo "[Success] Generating the examples from the schema produce the same files"
fi
