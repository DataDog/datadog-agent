#!/bin/bash

# Usage: ./run_llm_analysis.sh <prefix> <output_dir> [num_runs]
# Example: ./run_llm_analysis.sh memory-leak-export memory-leak-gpt-interpret
# Example: ./run_llm_analysis.sh memory-leak-export memory-leak-gpt-interpret 10

PREFIX="$1"
DIR="$2"
NUM_RUNS="${3:-10}"  # Default to 10 runs

if [ -z "$PREFIX" ] || [ -z "$DIR" ]; then
    echo "Usage: ./run_llm_analysis.sh <json_prefix> <output_dir> [num_runs]"
    echo "Example: ./run_llm_analysis.sh memory-leak-export memory-leak-gpt-interpret"
    echo "Example: ./run_llm_analysis.sh memory-leak-export memory-leak-gpt-interpret 10"
    exit 1
fi

if [ -z "$OPENAI_API_KEY" ]; then
    echo "Error: OPENAI_API_KEY not set"
    echo "  export OPENAI_API_KEY='sk-...'"
    exit 1
fi

mkdir -p "$DIR"

# Define combinations
COMBINATIONS=(
    "cusum:graphsketch:--cusum --graphsketch"
    "cusum:timecluster:--cusum --timecluster"
    "lightesd:graphsketch:--lightesd --graphsketch"
    "lightesd:timecluster:--lightesd --timecluster"
)

TOTAL=$((${#COMBINATIONS[@]} * NUM_RUNS))
CURRENT=0

echo "========================================"
echo "Running LLM analysis for $PREFIX"
echo "Combinations: ${#COMBINATIONS[@]}"
echo "Runs per combination: $NUM_RUNS"
echo "Total runs: $TOTAL"
echo "Output directory: $DIR"
echo "========================================"
echo ""

for combo in "${COMBINATIONS[@]}"; do
    # Parse combination string
    IFS=':' read -r detector correlator flags <<< "$combo"
    
    JSON_FILE="${PREFIX}_${detector}_${correlator}.json"
    
    if [ ! -f "$JSON_FILE" ]; then
        echo "Warning: $JSON_FILE not found, skipping..."
        continue
    fi
    
    echo "----------------------------------------"
    echo "Combination: ${detector} + ${correlator}"
    echo "Input: $JSON_FILE"
    echo "----------------------------------------"
    
    for run in $(seq 1 $NUM_RUNS); do
        CURRENT=$((CURRENT + 1))
        OUTPUT_FILE="$DIR/${PREFIX}_${detector}_${correlator}-gpt52-run${run}.txt"
        
        echo "  [$CURRENT/$TOTAL] Run $run/$NUM_RUNS -> $(basename $OUTPUT_FILE)"
        
        python3 analyze_with_llm.py "$JSON_FILE" $flags > "$OUTPUT_FILE" 2>&1
        
        # Rate limit: small delay between API calls
        sleep 1
    done
    
    echo ""
done

echo "========================================"
echo "Done! Results in $DIR/"
echo "========================================"
echo ""
echo "Files created:"
ls -la "$DIR/"*.txt 2>/dev/null | wc -l
echo " total files"
echo ""
echo "To evaluate results:"
echo "  ./run_evaluation.sh <scenario> $DIR/"
