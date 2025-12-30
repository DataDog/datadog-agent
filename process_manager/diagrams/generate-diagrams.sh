#!/bin/bash
#
# Generate high-quality diagrams from Mermaid (.mmd) files
#
# Usage: ./generate-diagrams.sh [OPTIONS] [filename.mmd]
#   --png     Generate PNG files (best for Google Docs, presentations)
#   --svg     Generate SVG files (default, best for web/scaling)
#   --help    Show help message
#
#   If no filename is provided, all .mmd files in the directory will be processed.
#
# Requirements:
#   Node.js and npm (uses npx to run mermaid-cli)
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="${SCRIPT_DIR}/mermaid.config.json"

# Quality settings
SCALE=3
WIDTH=1600
BG_COLOR="white"
OUTPUT_FORMAT="svg"  # Default format, can be overridden with --png flag

# Use global mmdc if available, otherwise use npx
if command -v mmdc &> /dev/null; then
    MMDC_CMD="mmdc"
else
    MMDC_CMD="npx --yes @mermaid-js/mermaid-cli"
fi

generate_diagram() {
    local input_file="$1"
    local output_file="${input_file%.mmd}.${OUTPUT_FORMAT}"
    
    echo "Generating: $(basename "$output_file")"
    
    $MMDC_CMD \
        --input "$input_file" \
        --output "$output_file" \
        --configFile "$CONFIG_FILE" \
        --scale "$SCALE" \
        --width "$WIDTH" \
        --backgroundColor "$BG_COLOR" \
        --quiet
    
    echo "  ✓ Created: $output_file"
}

# Check if npm/npx is available
if ! command -v npx &> /dev/null; then
    echo "Error: npm/npx is not installed."
    echo ""
    echo "Please install Node.js from https://nodejs.org"
    echo ""
    exit 1
fi

# Check if config file exists
if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "Warning: Config file not found at $CONFIG_FILE"
    echo "Using default mermaid settings."
    CONFIG_FILE=""
fi

cd "$SCRIPT_DIR"

# Parse arguments
INPUT_FILE=""
while [[ $# -gt 0 ]]; do
    case $1 in
        --png)
            OUTPUT_FORMAT="png"
            SCALE=4  # Higher scale for PNG
            shift
            ;;
        --svg)
            OUTPUT_FORMAT="svg"
            shift
            ;;
        --help|-h)
            echo "Usage: ./generate-diagrams.sh [OPTIONS] [filename.mmd]"
            echo ""
            echo "Options:"
            echo "  --png     Generate PNG files (best for Google Docs)"
            echo "  --svg     Generate SVG files (default)"
            echo "  --help    Show this help message"
            echo ""
            echo "Examples:"
            echo "  ./generate-diagrams.sh                    # Generate all as SVG"
            echo "  ./generate-diagrams.sh --png              # Generate all as PNG"
            echo "  ./generate-diagrams.sh --png diagram.mmd  # Generate single PNG"
            exit 0
            ;;
        *)
            INPUT_FILE="$1"
            shift
            ;;
    esac
done

if [[ -n "$INPUT_FILE" ]]; then
    # Generate single file
    if [[ -f "$INPUT_FILE" ]]; then
        generate_diagram "$INPUT_FILE"
    elif [[ -f "${SCRIPT_DIR}/$INPUT_FILE" ]]; then
        generate_diagram "${SCRIPT_DIR}/$INPUT_FILE"
    else
        echo "Error: File not found: $INPUT_FILE"
        exit 1
    fi
else
    # Generate all .mmd files
    FORMAT_UPPER=$(echo "$OUTPUT_FORMAT" | tr '[:lower:]' '[:upper:]')
    echo "Generating $FORMAT_UPPER files for all Mermaid diagrams..."
    echo ""
    
    count=0
    for mmd_file in *.mmd; do
        if [[ -f "$mmd_file" ]]; then
            generate_diagram "$mmd_file"
            ((count++))
        fi
    done
    
    echo ""
    echo "Done! Generated $count diagram(s) as $FORMAT_UPPER."
fi

