#!/bin/bash
# Simple script to analyze PII benchmark results
# Usage: ./scripts/analyze_pii_benchmarks.sh [benchstat_file]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BENCHMARK_DIR="${PROJECT_ROOT}/benchmarks/pii"

# Use provided file or find the latest benchstat file
if [[ -n "$1" ]]; then
    FILE="$1"
else
    FILE=$(find "${BENCHMARK_DIR}" -name "benchstat_*.txt" -type f | sort -r | head -1)
fi

if [[ ! -f "${FILE}" ]]; then
    echo "Error: No benchstat file found"
    exit 1
fi

echo "Analyzing: ${FILE##*/}"
echo "========================================"

# Extract performance data and organize by scenario
echo ""
echo "PERFORMANCE COMPARISON"
echo "======================"
echo ""

# Process the file to extract and organize data
awk '
BEGIN {
    section = ""
}

# Detect section headers
/sec\/op/ { section = "time"; next }
/B\/op/ { section = "memory"; next }
/allocs\/op/ { section = "allocs"; next }

# Process benchmark lines
/PIIDetectionComparison/ {
    # Parse the benchmark name
    name = $1
    split(name, parts, "/")

    if (length(parts) >= 3) {
        scenario = parts[2]
        approach = parts[3]

        # Store the value based on current section
        value = $2

        if (section == "time") {
            time[scenario, approach] = value
            scenarios[scenario] = 1
            approaches[approach] = 1
        }
        else if (section == "memory") {
            memory[scenario, approach] = value
        }
        else if (section == "allocs") {
            allocs[scenario, approach] = value
        }
    }
}

END {
    # Sort approaches in a specific order
    approach_order[1] = "regex-16"
    approach_order[2] = "hybrid-16"
    approach_order[3] = "tokenization_only-16"

    # Print table header
    printf "%-30s | %12s | %12s | %12s | %10s | %10s\n", "Scenario", "Approach", "Time", "Memory", "Allocs", "Speedup from regex (regex/current_approach)"
    printf "%-30s-+-%12s-+-%12s-+-%12s-+-%10s-+-%10s\n", "------------------------------", "------------", "------------", "------------", "----------", "----------"

    # Process each scenario
    for (scenario in scenarios) {
        first = 1

        # Get regex baseline time for speedup calculation (normalize to nanoseconds)
        regex_time_str = time[scenario, "regex-16"]
        if (index(regex_time_str, "µ") > 0) {
            gsub(/µ/, "", regex_time_str)
            regex_time = (regex_time_str + 0) * 1000  # Convert µs to ns
        } else {
            gsub(/n/, "", regex_time_str)
            regex_time = regex_time_str + 0
        }

        for (i = 1; i <= 3; i++) {
            approach = approach_order[i]

            if ((scenario, approach) in time) {
                # Calculate speedup (normalize to nanoseconds)
                current_time_str = time[scenario, approach]
                if (index(current_time_str, "µ") > 0) {
                    gsub(/µ/, "", current_time_str)
                    current_time = (current_time_str + 0) * 1000  # Convert µs to ns
                } else {
                    gsub(/n/, "", current_time_str)
                    current_time = current_time_str + 0
                }

                speedup = ""
                if (approach != "regex-16" && regex_time > 0) {
                    speedup_val = regex_time / current_time
                    speedup = sprintf("%.2fx", speedup_val)
                }

                # Clean up approach name
                display_approach = approach
                gsub(/-16/, "", display_approach)

                # Format scenario name (only show once per scenario group)
                scenario_name = (first) ? scenario : ""
                first = 0

                printf "%-30s | %12s | %12s | %12s | %10s | %10s\n",
                    scenario_name, display_approach,
                    time[scenario, approach],
                    memory[scenario, approach],
                    allocs[scenario, approach],
                    speedup
            }
        }
        print ""
    }
}
' "${FILE}"

echo ""
echo "SUMMARY STATISTICS"
echo "=================="
echo ""

# Calculate summary statistics
awk '
BEGIN {
    section = ""
}

/sec\/op/ { section = "time"; next }
/B\/op/ { section = "memory"; next }
/allocs\/op/ { section = "allocs"; next }

/PIIDetectionComparison/ {
    name = $1
    split(name, parts, "/")

    if (length(parts) >= 3) {
        scenario = parts[2]
        approach = parts[3]
        value = $2

        if (section == "time") {
            # Extract numeric value for time (normalize to nanoseconds)
            time_val = value
            if (index(time_val, "µ") > 0) {
                gsub(/µ/, "", time_val)
                time_val = (time_val + 0) * 1000  # Convert µs to ns
            } else {
                gsub(/n/, "", time_val)
                time_val = time_val + 0
            }

            if (approach == "regex-16") {
                regex_times[scenario] = time_val
                regex_count++
                regex_total += time_val
            }
            else if (approach == "hybrid-16") {
                hybrid_times[scenario] = time_val
                hybrid_count++
                hybrid_total += time_val
            }
        }
    }
}

END {
    # Calculate averages
    if (regex_count > 0) regex_avg = regex_total / regex_count
    if (hybrid_count > 0) hybrid_avg = hybrid_total / hybrid_count

    # Calculate speedups
    speedup_total = 0
    speedup_count = 0
    min_speedup = 999999
    max_speedup = 0
    min_scenario = ""
    max_scenario = ""

    for (scenario in regex_times) {
        if (scenario in hybrid_times) {
            speedup = regex_times[scenario] / hybrid_times[scenario]
            speedup_total += speedup
            speedup_count++

            if (speedup < min_speedup) {
                min_speedup = speedup
                min_scenario = scenario
            }
            if (speedup > max_speedup) {
                max_speedup = speedup
                max_scenario = scenario
            }
        }
    }

    avg_speedup = speedup_total / speedup_count

    # Convert back to microseconds for display
    regex_avg_us = regex_avg / 1000
    hybrid_avg_us = hybrid_avg / 1000

    printf "Average regex time:        %.2fµs\n", regex_avg_us
    printf "Average hybrid time:       %.2fµs\n", hybrid_avg_us
    printf "\n"
    printf "Hybrid speedup (average):  %.2fx faster\n", avg_speedup
    printf "Hybrid speedup (minimum):  %.2fx faster  [%s]\n", min_speedup, min_scenario
    printf "Hybrid speedup (maximum):  %.2fx faster  [%s]\n", max_speedup, max_scenario
    printf "\n"
    printf "Overall time reduction:    %.1f%%\n", (1 - (hybrid_avg / regex_avg)) * 100
}
' "${FILE}"