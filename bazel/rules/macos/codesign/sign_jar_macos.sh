#!/bin/bash
# Sign binaries and libraries inside a JAR file
# Usage: sign_jar.sh <input_jar> <output_jar> <signing_identity> [entitlements_file]

set -e

INPUT_JAR="$1"
OUTPUT_JAR="$2"
SIGNING_IDENTITY="$3"
ENTITLEMENTS_FILE="${4:-}"

# Validate inputs
if [[ -z "$INPUT_JAR" || -z "$OUTPUT_JAR" || -z "$SIGNING_IDENTITY" ]]; then
    echo "Usage: $0 <input_jar> <output_jar> <signing_identity> [entitlements_file]" >&2
    exit 1
fi

if [[ ! -f "$INPUT_JAR" ]]; then
    echo "Error: Input JAR not found: $INPUT_JAR" >&2
    exit 1
fi

if [[ -n "$ENTITLEMENTS_FILE" && ! -f "$ENTITLEMENTS_FILE" ]]; then
    echo "Error: Entitlements file not found: $ENTITLEMENTS_FILE" >&2
    exit 1
fi

# Convert OUTPUT_JAR to absolute path if it's relative
if [[ ! "$OUTPUT_JAR" = /* ]]; then
    OUTPUT_JAR="$(pwd)/$OUTPUT_JAR"
fi

# Create a temporary directory for unpacking
TEMP_DIR=$(mktemp -d)
trap "rm -rf '$TEMP_DIR'" EXIT

# Extract jar to temp directory
unzip -q "$INPUT_JAR" -d "$TEMP_DIR"

# Build codesign command options
CODESIGN_OPTS=(--force --timestamp --deep -s "$SIGNING_IDENTITY")
if [[ -n "$ENTITLEMENTS_FILE" ]]; then
    CODESIGN_OPTS+=(-o runtime --entitlements "$ENTITLEMENTS_FILE")
fi

# Find and sign all binary/library files
# Search for .so, .dylib, and .jnilib files
if find "$TEMP_DIR" -type f \( -name "*.so" -o -name "*.dylib" -o -name "*.jnilib" \) | grep -q .; then
    echo "Signing native libraries in JAR with: codesign ${CODESIGN_OPTS[@]}"
    find . -type f \( -name "*.so" -o -name "*.dylib" -o -name "*.jnilib" \) | while read -r file; do
        echo "  Signing: $file"
        codesign "${CODESIGN_OPTS[@]}" "$file"
    done
else
    echo "No native libraries found to sign"
fi

# Create output directory if needed
mkdir -p "$(dirname "$OUTPUT_JAR")"

# Create new signed jar, preserving the original structure
cd "$TEMP_DIR"
zip -q -r "$OUTPUT_JAR" .

# Set permissions on output jar
chmod 0644 "$OUTPUT_JAR"

echo "Signed JAR created: $OUTPUT_JAR"
