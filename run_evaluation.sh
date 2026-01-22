#!/bin/bash
# Batch evaluate diagnosis results against ground truth
#
# Usage:
#   ./run_evaluation.sh <scenario> <directory>
#
# Examples:
#   ./run_evaluation.sh memory-leak memory-leak-gpt-interpret/
#   ./run_evaluation.sh network-latency network-latency-gpt-interpret/

set -e

SCENARIO="${1:-}"
DIR="${2:-}"

if [ -z "$SCENARIO" ] || [ -z "$DIR" ]; then
    echo "Usage: $0 <scenario> <directory>"
    echo ""
    echo "Scenarios:"
    echo "  memory-leak      - Memory leak causing OOM kills"
    echo "  network-latency  - Network delay on Redis pod"
    echo ""
    echo "Examples:"
    echo "  $0 memory-leak memory-leak-gpt-interpret/"
    echo "  $0 network-latency network-latency-gpt-interpret/"
    exit 1
fi

if [ -z "$OPENAI_API_KEY" ]; then
    echo "Error: OPENAI_API_KEY not set"
    echo "  export OPENAI_API_KEY='sk-...'"
    exit 1
fi

if [ ! -d "$DIR" ]; then
    echo "Error: Directory not found: $DIR"
    exit 1
fi

# Create output directory
OUTDIR="${DIR}/evaluations"
mkdir -p "$OUTDIR"

# Count diagnosis files
NUM_FILES=$(find "$DIR" -maxdepth 1 -name "*.txt" -type f | wc -l | tr -d ' ')

echo "========================================"
echo "Batch Evaluation: $SCENARIO"
echo "Directory: $DIR"
echo "Files to evaluate: $NUM_FILES"
echo "Output: $OUTDIR"
echo "========================================"
echo ""

CURRENT=0
SCORES_FILE="$OUTDIR/_scores.csv"
echo "file,identified,score" > "$SCORES_FILE"

# Find all diagnosis files (*.txt files that contain "RESPONSE:")
for file in "$DIR"/*.txt; do
    if [ ! -f "$file" ]; then
        continue
    fi
    
    # Skip if not a diagnosis file (should contain "RESPONSE:")
    if ! grep -q "RESPONSE:" "$file" 2>/dev/null; then
        echo "Skipping (not a diagnosis file): $(basename $file)"
        continue
    fi
    
    CURRENT=$((CURRENT + 1))
    basename=$(basename "$file" .txt)
    outfile="$OUTDIR/${basename}-eval.txt"
    
    echo "[$CURRENT/$NUM_FILES] Evaluating: $basename"
    
    python3 evaluate_diagnosis.py "$file" --scenario "$SCENARIO" > "$outfile" 2>&1
    
    # Extract scores for CSV (new simpler format)
    identified=$(grep -oE "Identified Problem[^:]*:[^a-z]*\[?(yes|partial|no)" "$outfile" | grep -oE "(yes|partial|no)" | head -1 || echo "")
    score=$(grep -oE "Score[^:]*:[^0-9]*\[?([0-9]+)" "$outfile" | grep -oE "[0-9]+" | head -1 || echo "")
    
    echo "$basename,$identified,$score" >> "$SCORES_FILE"
    
    echo "  Identified: $identified | Score: $score"
    
    # Rate limit: wait a bit between API calls
    sleep 1
done

echo ""
echo "========================================"
echo "Evaluation complete!"
echo "========================================"
echo ""

# Generate summary by combination
echo "SUMMARY BY COMBINATION"
echo "========================================"

# Parse CSV and aggregate by combination (detector_correlator)
echo ""
printf "%-25s %6s %10s %6s %8s %6s\n" "Combination" "Runs" "AvgScore" "PASS" "PARTIAL" "FAIL"
printf "%-25s %6s %10s %6s %8s %6s\n" "-------------------------" "------" "----------" "------" "--------" "------"

# Get unique combinations (remove run number suffix)
combinations=$(tail -n +2 "$SCORES_FILE" | cut -d',' -f1 | sed 's/-run[0-9]*$//' | sort -u)

for combo in $combinations; do
    # Get all rows for this combination
    rows=$(grep "^${combo}-run" "$SCORES_FILE" || grep "^${combo}," "$SCORES_FILE" || true)
    
    if [ -z "$rows" ]; then
        continue
    fi
    
    num_runs=$(echo "$rows" | wc -l | tr -d ' ')
    
    # Calculate average score and count identified
    total_score=0
    count=0
    id_yes=0
    id_partial=0
    id_no=0
    
    while IFS=',' read -r name identified score; do
        if [ -n "$score" ] && [ "$score" -eq "$score" ] 2>/dev/null; then
            total_score=$((total_score + score))
            count=$((count + 1))
        fi
        case "$identified" in
            yes) id_yes=$((id_yes + 1)) ;;
            partial) id_partial=$((id_partial + 1)) ;;
            no) id_no=$((id_no + 1)) ;;
        esac
    done <<< "$rows"
    
    if [ $count -gt 0 ]; then
        avg_score=$((total_score / count))
    else
        avg_score="N/A"
    fi
    
    # Shorten combo name for display
    short_combo=$(echo "$combo" | sed "s/.*_\(cusum\|lightesd\)_\(graphsketch\|timecluster\).*/\1+\2/")
    
    printf "%-25s %6s %8s %6s %6s %6s\n" "$short_combo" "$num_runs" "$avg_score" "$id_yes" "$id_partial" "$id_no"
done

echo ""
echo "Results saved to: $OUTDIR"
echo "Scores CSV: $SCORES_FILE"
echo ""
echo "To view detailed evaluation for a specific file:"
echo "  cat $OUTDIR/<filename>-eval.txt"
