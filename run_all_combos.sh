#!/bin/bash
# Run all detector + correlator combinations and export results
# Usage: ./run_all_combos.sh <parquet_file_or_dir>
# Example: ./run_all_combos.sh test_data/memory-leak-export.parquet

set -e

if [ -z "$1" ]; then
  echo "Usage: $0 <parquet_file_or_dir>"
  echo "Example: $0 test_data/memory-leak-export.parquet"
  exit 1
fi

DATA="$1"
DEMO="/tmp/observer-demo-v2"

# Extract base name for output files (remove path and extension)
BASENAME=$(basename "$DATA" .parquet)

echo "Building latest demo..."
cd /Users/ella.taira/Desktop/datadog-agent
go build -o "$DEMO" ./cmd/observer-demo-v2

echo ""
echo "=== Running all 4 combinations for: $DATA ==="
echo "=== Output prefix: ${BASENAME}_ ==="
echo ""

# 1. CUSUM + GraphSketchCorrelator
echo "[1/4] CUSUM + GraphSketchCorrelator..."
"$DEMO" -parquet "$DATA" -all -cusum -graphsketch-correlator -time-cluster=false -output "${BASENAME}_cusum_graphsketch.json"
echo "  -> ${BASENAME}_cusum_graphsketch.json"
echo ""

# 2. LightESD + GraphSketchCorrelator
echo "[2/4] LightESD + GraphSketchCorrelator..."
"$DEMO" -parquet "$DATA" -all -lightesd -graphsketch-correlator -time-cluster=false -output "${BASENAME}_lightesd_graphsketch.json"
echo "  -> ${BASENAME}_lightesd_graphsketch.json"
echo ""

# 3. CUSUM + TimeClusterCorrelator
echo "[3/4] CUSUM + TimeClusterCorrelator..."
"$DEMO" -parquet "$DATA" -all -cusum -time-cluster -graphsketch-correlator=false -output "${BASENAME}_cusum_timecluster.json"
echo "  -> ${BASENAME}_cusum_timecluster.json"
echo ""

# 4. LightESD + TimeClusterCorrelator
echo "[4/4] LightESD + TimeClusterCorrelator..."
"$DEMO" -parquet "$DATA" -all -lightesd -time-cluster -graphsketch-correlator=false -output "${BASENAME}_lightesd_timecluster.json"
echo "  -> ${BASENAME}_lightesd_timecluster.json"
echo ""

echo "=== All done! ==="
echo ""
echo "Results summary:"
for f in ${BASENAME}_*.json; do
  echo "--- $f ---"
  jq '{detector, correlator, total_anomalies, unique_sources_in_anomalies, total_correlations, total_edges}' "$f"
  
  # Show top 3 edges for GraphSketch results
  if jq -e '.edges | length > 0' "$f" > /dev/null 2>&1; then
    echo "  Top 3 edges:"
    jq -r '.edges[:3][] | "    \(.source1) ↔ \(.source2) (obs: \(.observations), freq: \(.frequency | . * 100 | round / 100))"' "$f"
  fi
  echo ""
done
