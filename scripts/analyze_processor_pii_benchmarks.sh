#!/bin/bash
# Analyzer for Processor PII Mode Benchmark Results
# Usage: ./scripts/analyze_processor_pii_benchmarks.sh <benchmark_results_file>

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BENCHMARK_DIR="${PROJECT_ROOT}/benchmarks/pii_processor"

# Use provided file or find the latest benchmark file
if [[ -n "$1" ]]; then
    FILE="$1"
else
    FILE=$(find "${BENCHMARK_DIR}" -name "processor_modes_*.txt" -type f | sort -r | head -1)
fi

if [[ ! -f "${FILE}" ]]; then
    echo "Error: No benchmark file found"
    echo "Usage: $0 [benchmark_file]"
    exit 1
fi

FILENAME=$(basename "${FILE}")
TIMESTAMP="${FILENAME#processor_modes_}"
TIMESTAMP="${TIMESTAMP%.txt}"

echo "Analyzing: ${FILENAME}"
echo "========================================"
echo ""

# Extract separate files for benchstat
TEMP_DIR=$(mktemp -d)
trap 'rm -rf ${TEMP_DIR}' EXIT

# Filter and clean the benchmark data
# Combine benchmark names with their result lines (benchstat format)
awk '
    # Keep header lines
    /^goos:/ || /^goarch:/ || /^pkg:/ || /^cpu:/ { print; next }

    # Extract benchmark name from lines that have it (before the tab and debug log)
    /^BenchmarkProcessorPIIModes/ {
        current_benchmark = $1
        next
    }

    # When we see a result line, combine it with the previous benchmark name
    /^[[:space:]]*[0-9]+[[:space:]]+[0-9]+(\.[0-9]+)? ns\/op/ {
        if (current_benchmark != "") {
            # Combine benchmark name and results on one line (benchstat format)
            print current_benchmark " " $0
            current_benchmark = ""
        }
    }
' "${FILE}" > "${TEMP_DIR}/clean.txt"

# Now split by mode
grep '/regex-16' "${TEMP_DIR}/clean.txt" > "${TEMP_DIR}/regex.txt" || true
grep '/hybrid-16' "${TEMP_DIR}/clean.txt" > "${TEMP_DIR}/hybrid.txt" || true
grep '/disabled-16' "${TEMP_DIR}/clean.txt" > "${TEMP_DIR}/disabled.txt" || true

# Check if we have data
if [[ ! -s "${TEMP_DIR}/regex.txt" ]] || [[ ! -s "${TEMP_DIR}/hybrid.txt" ]]; then
    echo "Error: Could not extract regex or hybrid benchmark data"
    exit 1
fi

# Add headers to the files for benchstat
for mode_file in "${TEMP_DIR}/regex.txt" "${TEMP_DIR}/hybrid.txt" "${TEMP_DIR}/disabled.txt"; do
    if [[ -s "${mode_file}" ]]; then
        # Extract header info from original file
        head -6 "${FILE}" | grep -E '^(goos|goarch|pkg|cpu):' > "${mode_file}.tmp"
        cat "${mode_file}" >> "${mode_file}.tmp"
        mv "${mode_file}.tmp" "${mode_file}"
    fi
done

echo "Running benchstat analysis..."
echo ""

# Run benchstat comparison: regex vs hybrid
benchstat "${TEMP_DIR}/regex.txt" "${TEMP_DIR}/hybrid.txt" > "${TEMP_DIR}/benchstat.txt"

# Save the benchstat output for reference
cp "${TEMP_DIR}/benchstat.txt" "${BENCHMARK_DIR}/benchstat_${TIMESTAMP}.txt"

echo "PERFORMANCE COMPARISON"
echo "======================"
echo ""

# Parse benchstat output to create a unified table
awk -v disabled_file="${TEMP_DIR}/disabled.txt" '
BEGIN {
    section = ""

    # Read disabled mode data
    while ((getline < disabled_file) > 0) {
        if (/^BenchmarkProcessorPIIModes/) {
            split($1, parts, "/")
            if (length(parts) >= 3) {
                scenario = parts[2]
                disabled_time[scenario] = $2 " " $3
                disabled_mem[scenario] = $4 " " $5
                disabled_allocs[scenario] = $6 " " $7
                disabled_time_ns[scenario] = $2 + 0
            }
        }
    }
    close(disabled_file)
}

# Detect section headers in benchstat output
/sec\/op/ { section = "time"; next }
/B\/op/ { section = "memory"; next }
/allocs\/op/ { section = "allocs"; next }

# Skip geomean and notes
/^geomean/ || /^¹/ || /^²/ { next }

# Parse benchmark lines - benchstat shows regex and hybrid in separate rows
/^ProcessorPIIModes/ {
    line = $0
    name = $1
    split(name, parts, "/")

    if (length(parts) >= 3) {
        scenario = parts[2]
        mode = parts[3]

        # Determine if this is a FullMessage benchmark
        if (index(name, "FullMessage") > 0) {
            is_full_message = 1
            full_message_scenarios[scenario] = 1
        } else {
            is_full_message = 0
            regular_scenarios[scenario] = 1
        }

        # Extract the value portion of the line (everything after the benchmark name)
        # The benchmark name ends around column 60, values appear later
        remainder = substr(line, length(name) + 1)

        # Remove leading whitespace and get the next value
        gsub(/^[[:space:]]*/, "", remainder)

        # Extract just the value (number + unit + optional ±uncertainty)
        # Match patterns like "8.127µ" or "1.419Ki" or "16.00"
        if (match(remainder, /[0-9]+\.?[0-9]*(µ|Ki|Mi)?/)) {
            value = substr(remainder, RSTART, RLENGTH)

            if (section == "time") {
                if (mode == "regex-16") {
                    regex_time[scenario] = value
                    regex_is_full[scenario] = is_full_message
                } else if (mode == "hybrid-16") {
                    hybrid_time[scenario] = value
                    hybrid_is_full[scenario] = is_full_message
                }
            }
            else if (section == "memory") {
                if (mode == "regex-16") {
                    regex_mem[scenario] = value
                } else if (mode == "hybrid-16") {
                    hybrid_mem[scenario] = value
                }
            }
            else if (section == "allocs") {
                if (mode == "regex-16") {
                    regex_allocs[scenario] = value
                } else if (mode == "hybrid-16") {
                    hybrid_allocs[scenario] = value
                }
            }
        }
    }
}

END {
    # Print table header for regular benchmarks
    print "REGULAR BENCHMARKS"
    print "------------------"
    print ""
    printf "%-30s | %15s | %12s | %12s | %10s | %10s\n", "Scenario", "Mode", "Time", "Memory", "Allocs", "Speedup"
    printf "%-30s-+-%15s-+-%12s-+-%12s-+-%10s-+-%10s\n", "------------------------------", "---------------", "------------", "------------", "----------", "----------"

    # Process regular scenarios
    for (scenario in regular_scenarios) {
        # Parse regex time to get numeric value for speedup calculation
        regex_time_str = regex_time[scenario]
        gsub(/[^0-9.]/, "", regex_time_str)
        regex_ns = regex_time_str + 0
        # Convert µs to ns if needed
        if (index(regex_time[scenario], "µ") > 0) {
            regex_ns = regex_ns * 1000
        }

        # Regex mode
        printf "%-30s | %15s | %12s | %12s | %10s | %10s\n",
            scenario, "regex",
            regex_time[scenario],
            regex_mem[scenario],
            regex_allocs[scenario],
            ""

        # Hybrid mode
        hybrid_time_str = hybrid_time[scenario]
        gsub(/[^0-9.]/, "", hybrid_time_str)
        hybrid_ns = hybrid_time_str + 0
        if (index(hybrid_time[scenario], "µ") > 0) {
            hybrid_ns = hybrid_ns * 1000
        }

        speedup_hybrid = ""
        if (regex_ns > 0 && hybrid_ns > 0) {
            speedup_hybrid = sprintf("%.2fx", regex_ns / hybrid_ns)
        }

        printf "%-30s | %15s | %12s | %12s | %10s | %10s\n",
            "", "hybrid",
            hybrid_time[scenario],
            hybrid_mem[scenario],
            hybrid_allocs[scenario],
            speedup_hybrid

        # Disabled mode (if available)
        if (scenario in disabled_time) {
            disabled_ns = disabled_time_ns[scenario]
            speedup_disabled = ""
            if (regex_ns > 0 && disabled_ns > 0) {
                speedup_disabled = sprintf("%.2fx", regex_ns / disabled_ns)
            }

            printf "%-30s | %15s | %12s | %12s | %10s | %10s\n",
                "", "disabled",
                disabled_time[scenario],
                disabled_mem[scenario],
                disabled_allocs[scenario],
                speedup_disabled
        }

        print ""
    }

    # Print separator and header for FullMessage benchmarks
    print ""
    print "============================================================================"
    print ""
    print "FULL MESSAGE BENCHMARKS"
    print "-----------------------"
    print ""
    printf "%-30s | %15s | %12s | %12s | %10s | %10s\n", "Scenario", "Mode", "Time", "Memory", "Allocs", "Speedup"
    printf "%-30s-+-%15s-+-%12s-+-%12s-+-%10s-+-%10s\n", "------------------------------", "---------------", "------------", "------------", "----------", "----------"

    # Process FullMessage scenarios
    for (scenario in full_message_scenarios) {
        # Parse regex time to get numeric value for speedup calculation
        regex_time_str = regex_time[scenario]
        gsub(/[^0-9.]/, "", regex_time_str)
        regex_ns = regex_time_str + 0
        # Convert µs to ns if needed
        if (index(regex_time[scenario], "µ") > 0) {
            regex_ns = regex_ns * 1000
        }

        # Regex mode
        printf "%-30s | %15s | %12s | %12s | %10s | %10s\n",
            scenario, "regex",
            regex_time[scenario],
            regex_mem[scenario],
            regex_allocs[scenario],
            ""

        # Hybrid mode
        hybrid_time_str = hybrid_time[scenario]
        gsub(/[^0-9.]/, "", hybrid_time_str)
        hybrid_ns = hybrid_time_str + 0
        if (index(hybrid_time[scenario], "µ") > 0) {
            hybrid_ns = hybrid_ns * 1000
        }

        speedup_hybrid = ""
        if (regex_ns > 0 && hybrid_ns > 0) {
            speedup_hybrid = sprintf("%.2fx", regex_ns / hybrid_ns)
        }

        printf "%-30s | %15s | %12s | %12s | %10s | %10s\n",
            "", "hybrid",
            hybrid_time[scenario],
            hybrid_mem[scenario],
            hybrid_allocs[scenario],
            speedup_hybrid

        # Disabled mode (if available)
        if (scenario in disabled_time) {
            disabled_ns = disabled_time_ns[scenario]
            speedup_disabled = ""
            if (regex_ns > 0 && disabled_ns > 0) {
                speedup_disabled = sprintf("%.2fx", regex_ns / disabled_ns)
            }

            printf "%-30s | %15s | %12s | %12s | %10s | %10s\n",
                "", "disabled",
                disabled_time[scenario],
                disabled_mem[scenario],
                disabled_allocs[scenario],
                speedup_disabled
        }

        print ""
    }
}
' "${TEMP_DIR}/benchstat.txt"

echo ""
echo "SUMMARY STATISTICS"
echo "=================="
echo ""

# Calculate summary statistics from benchstat output
awk '
BEGIN {
    section = ""
    regex_count = 0
    hybrid_count = 0
    regex_total = 0
    hybrid_total = 0
}

/sec\/op/ { section = "time"; next }
/B\/op/ { section = ""; next }
/allocs\/op/ { section = ""; next }

# Skip geomean and notes
/^geomean/ || /^¹/ || /^²/ { next }

/^ProcessorPIIModes/ {
    if (section == "time") {
        line = $0
        name = $1
        split(name, parts, "/")

        if (length(parts) >= 3) {
            scenario = parts[2]
            mode = parts[3]

            # Extract the value portion of the line
            remainder = substr(line, length(name) + 1)
            gsub(/^[[:space:]]*/, "", remainder)

            # Extract the time value
            if (match(remainder, /[0-9]+\.?[0-9]*µ/)) {
                time_with_unit = substr(remainder, RSTART, RLENGTH)

                # Parse time value
                time_str = time_with_unit
                gsub(/[^0-9.]/, "", time_str)
                time_val = time_str + 0

                # Convert µs to ns
                if (index(time_with_unit, "µ") > 0) {
                    time_val = time_val * 1000
                }

                if (mode == "regex-16") {
                    regex_times[scenario] = time_val
                    regex_count++
                    regex_total += time_val
                } else if (mode == "hybrid-16") {
                    hybrid_times[scenario] = time_val
                    hybrid_count++
                    hybrid_total += time_val
                }
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

    if (speedup_count > 0) {
        avg_speedup = speedup_total / speedup_count

        printf "Average regex time:        %.0f ns/op\n", regex_avg
        printf "Average hybrid time:       %.0f ns/op\n", hybrid_avg
        printf "\n"
        printf "Hybrid speedup (average):  %.2fx faster\n", avg_speedup
        printf "Hybrid speedup (minimum):  %.2fx faster  [%s]\n", min_speedup, min_scenario
        printf "Hybrid speedup (maximum):  %.2fx faster  [%s]\n", max_speedup, max_scenario
        printf "\n"
        printf "Overall time reduction:    %.1f%%\n", (1 - (hybrid_avg / regex_avg)) * 100
    } else {
        print "No data available for comparison"
    }
}
' "${TEMP_DIR}/benchstat.txt"

echo ""
echo "Analysis complete!"
echo "Benchstat output saved to:"
echo "  ${BENCHMARK_DIR}/benchstat_${TIMESTAMP}.txt"
echo ""
