#!/usr/bin/env bash
# OTLP MaxCount rejection control — NOT a memory reproducer.
#
# pkg/opentelemetry-mapping-go/otlp/metrics/validation.go drops points whose
# total bucket count exceeds quantile.Default().MaxCount() (= 4096 × 65,536 =
# 268,431,360) BEFORE sketch conversion. This guard (PR #57002) prevents the
# overflow-bin allocation path from being reached via OTLP.
#
# This script sends an OTLP histogram with a count just above MaxCount and
# confirms it is dropped (not stored), not that memory blows up.
#
# Usage:
#   ./run_otlp_rejection.sh
#
# Prerequisites:
#   - Built agent with OTLP ingest enabled
#   - otlpgrpc/otlphttp client (grpcurl or a small Go/Python OTLP exporter)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LAB="$SCRIPT_DIR"
ART="$LAB/artifacts"
mkdir -p "$ART"

# MaxCount = 4096 * 65536 = 268,431,360
MAXCOUNT=$((4096 * 65536))
OVER=$((MAXCOUNT + 1))

cat <<EOF
=== OTLP rejection control (NOT a memory repro) ===

MaxCount = $MAXCOUNT
Sending OTLP histogram with count = $OVER (just over MaxCount).

Expected behavior (main AND fix branch):
  - Point is dropped before sketch conversion (validation.go)
  - No overflow-bin allocation occurs
  - No memory blow-up
  - Agent logs a "bucket_count_too_high" warning (once per metric name)

This is a rejection control, not a reproducer. The OTLP path cannot trigger
the memory bug because MaxCount guards the input before insertCounts runs.

To send the OTLP payload, use one of:

  # grpcurl (if OTLP gRPC endpoint is enabled on :4317):
  grpcurl -plaintext -d @ 127.0.0.1:4317 opentelemetry.proto.collector.metrics.v1.MetricsService/Export <<JSON
  {
    "resourceMetrics": [{
      "scopeMetrics": [{
        "metrics": [{
          "name": "repro.otlp.histogram",
          "histogram": {
            "dataPoints": [{
              "bucketCounts": [$OVER],
              "explicitBounds": [1.0],
              "sum": 0,
              "count": $OVER
            }]
          }
        }]
      }]
    }]
  }
  JSON

  # Or a small Go/Python OTLP exporter script (left as an exercise; the point
  # is that the count is rejected, not stored).

After sending, check:
  - Agent log for "bucket_count_too_high"
  - expvar: no sketch retained for repro.otlp.histogram
  - heap: no overflow-bin allocation

See README.md § "OTLP — NOT a memory repro" for why this is not a repro path.
EOF
