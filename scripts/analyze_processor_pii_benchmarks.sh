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
echo "Extracting benchmark data..."
grep '/enabled-16' "${TEMP_DIR}/clean.txt" > "${TEMP_DIR}/enabled.txt" || true
grep '/disabled-16' "${TEMP_DIR}/clean.txt" > "${TEMP_DIR}/disabled.txt" || true

# Check if we have data
if [[ ! -s "${TEMP_DIR}/enabled.txt" ]]; then
    echo "Error: Could not extract enabled benchmark data (file empty or missing)"
    echo "Clean file contents:"
    head -20 "${TEMP_DIR}/clean.txt"
    exit 1
fi

if [[ ! -s "${TEMP_DIR}/disabled.txt" ]]; then
    echo "Error: Could not extract disabled benchmark data (file empty or missing)"
    echo "Clean file contents:"
    head -20 "${TEMP_DIR}/clean.txt"
    exit 1
fi

echo "Found $(wc -l < ${TEMP_DIR}/enabled.txt) enabled benchmarks and $(wc -l < ${TEMP_DIR}/disabled.txt) disabled benchmarks"

# Add headers to the files for benchstat
echo "Adding headers to benchmark files..."
for mode_file in "${TEMP_DIR}/enabled.txt" "${TEMP_DIR}/disabled.txt"; do
    if [[ -s "${mode_file}" ]]; then
        # Extract header info from original file
        if ! head -100 "${FILE}" | grep -E '^(goos|goarch|pkg|cpu):' > "${mode_file}.tmp"; then
            echo "Warning: Could not extract headers from ${FILE}"
        fi
        cat "${mode_file}" >> "${mode_file}.tmp"
        mv "${mode_file}.tmp" "${mode_file}"
    fi
done

echo "Headers added successfully"
echo "Running benchstat analysis..."
echo ""

# Run benchstat comparison: disabled (baseline) vs enabled
if ! benchstat "${TEMP_DIR}/disabled.txt" "${TEMP_DIR}/enabled.txt" > "${TEMP_DIR}/benchstat.txt"; then
    echo "Error: benchstat command failed"
    exit 1
fi

# Save the benchstat output for reference
mkdir -p "${BENCHMARK_DIR}"
if ! cp "${TEMP_DIR}/benchstat.txt" "${BENCHMARK_DIR}/benchstat_${TIMESTAMP}.txt"; then
    echo "Warning: Could not save benchstat output to ${BENCHMARK_DIR}/benchstat_${TIMESTAMP}.txt"
fi

# Create analysis output file and save analysis
ANALYSIS_FILE="${BENCHMARK_DIR}/analyze_processor_modes_${TIMESTAMP}.txt"

# Run analysis and save to both console and file
{
echo "PERFORMANCE COMPARISON (Disabled Baseline vs Enabled)"
echo "======================================================"
echo ""

# Parse benchstat output to create a unified table
awk '
BEGIN {
    section = ""
}

# Detect section headers in benchstat output
/sec\/op/ { section = "time"; next }
/B\/op/ { section = "memory"; next }
/allocs\/op/ { section = "allocs"; next }

# Skip geomean, notes, and header lines
/^geomean/ || /^¹/ || /^²/ || /^³/ || /^\|/ || /^[[:space:]]*$/ { next }

# Parse benchmark lines - benchstat shows disabled and enabled in separate rows
/ProcessorPIIModes/ {
    line = $0
    # Extract benchmark name (first field)
    match(line, /ProcessorPIIModes\/[^[:space:]]+/)
    name = substr(line, RSTART, RLENGTH)
    
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

        # Extract the value after the benchmark name
        remainder = substr(line, RSTART + RLENGTH)
        gsub(/^[[:space:]]*/, "", remainder)

        # Extract just the value (number + unit)
        # Match patterns like "391.9n" or "25.98µ" or "40.68Ki" or "5.000"
        if (match(remainder, /[0-9]+\.?[0-9]*(n|µ|Ki|Mi)?/)) {
            value = substr(remainder, RSTART, RLENGTH)

            if (section == "time") {
                if (mode == "disabled-16") {
                    disabled_time[scenario] = value
                    disabled_is_full[scenario] = is_full_message
                } else if (mode == "enabled-16") {
                    enabled_time[scenario] = value
                    enabled_is_full[scenario] = is_full_message
                }
            }
            else if (section == "memory") {
                if (mode == "disabled-16") {
                    disabled_mem[scenario] = value
                } else if (mode == "enabled-16") {
                    enabled_mem[scenario] = value
                }
            }
            else if (section == "allocs") {
                if (mode == "disabled-16") {
                    disabled_allocs[scenario] = value
                } else if (mode == "enabled-16") {
                    enabled_allocs[scenario] = value
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
    printf "%-30s | %15s | %12s | %12s | %10s | %10s\n", "Scenario", "Mode", "Time", "Memory", "Allocs", "Overhead"
    printf "%-30s-+-%15s-+-%12s-+-%12s-+-%10s-+-%10s\n", "------------------------------", "---------------", "------------", "------------", "----------", "----------"

    # Process regular scenarios
    for (scenario in regular_scenarios) {
        # Parse disabled time to get numeric value for overhead calculation
        disabled_time_str = disabled_time[scenario]
        gsub(/[^0-9.]/, "", disabled_time_str)
        disabled_ns = disabled_time_str + 0
        # Convert µs to ns if needed
        if (index(disabled_time[scenario], "µ") > 0) {
            disabled_ns = disabled_ns * 1000
        }

        # Disabled mode (baseline)
        printf "%-30s | %15s | %12s | %12s | %10s | %10s\n",
            scenario, "disabled",
            disabled_time[scenario],
            disabled_mem[scenario],
            disabled_allocs[scenario],
            ""

        # Enabled mode
        enabled_time_str = enabled_time[scenario]
        gsub(/[^0-9.]/, "", enabled_time_str)
        enabled_ns = enabled_time_str + 0
        if (index(enabled_time[scenario], "µ") > 0) {
            enabled_ns = enabled_ns * 1000
        }

        overhead_enabled = ""
        if (disabled_ns > 0 && enabled_ns > 0) {
            overhead_enabled = sprintf("%.1fx", enabled_ns / disabled_ns)
        }

        printf "%-30s | %15s | %12s | %12s | %10s | %10s\n",
            "", "enabled",
            enabled_time[scenario],
            enabled_mem[scenario],
            enabled_allocs[scenario],
            overhead_enabled

        print ""
    }

    # Print separator and header for FullMessage benchmarks
    if (length(full_message_scenarios) > 0) {
        print ""
        print "============================================================================"
        print ""
        print "FULL MESSAGE BENCHMARKS"
        print "-----------------------"
        print ""
        printf "%-30s | %15s | %12s | %12s | %10s | %10s\n", "Scenario", "Mode", "Time", "Memory", "Allocs", "Overhead"
        printf "%-30s-+-%15s-+-%12s-+-%12s-+-%10s-+-%10s\n", "------------------------------", "---------------", "------------", "------------", "----------", "----------"

        # Process FullMessage scenarios
        for (scenario in full_message_scenarios) {
            # Parse disabled time to get numeric value for overhead calculation
            disabled_time_str = disabled_time[scenario]
            gsub(/[^0-9.]/, "", disabled_time_str)
            disabled_ns = disabled_time_str + 0
            # Convert µs to ns if needed
            if (index(disabled_time[scenario], "µ") > 0) {
                disabled_ns = disabled_ns * 1000
            }

            # Disabled mode (baseline)
            printf "%-30s | %15s | %12s | %12s | %10s | %10s\n",
                scenario, "disabled",
                disabled_time[scenario],
                disabled_mem[scenario],
                disabled_allocs[scenario],
                ""

            # Enabled mode
            enabled_time_str = enabled_time[scenario]
            gsub(/[^0-9.]/, "", enabled_time_str)
            enabled_ns = enabled_time_str + 0
            if (index(enabled_time[scenario], "µ") > 0) {
                enabled_ns = enabled_ns * 1000
            }

            overhead_enabled = ""
            if (disabled_ns > 0 && enabled_ns > 0) {
                overhead_enabled = sprintf("%.1fx", enabled_ns / disabled_ns)
            }

            printf "%-30s | %15s | %12s | %12s | %10s | %10s\n",
                "", "enabled",
                enabled_time[scenario],
                enabled_mem[scenario],
                enabled_allocs[scenario],
                overhead_enabled

            print ""
        }
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
    disabled_count = 0
    enabled_count = 0
    disabled_total = 0
    enabled_total = 0
}

/sec\/op/ { section = "time"; next }
/B\/op/ { section = ""; next }
/allocs\/op/ { section = ""; next }

# Skip geomean, notes, and header lines
/^geomean/ || /^¹/ || /^²/ || /^³/ || /^\|/ || /^[[:space:]]*$/ { next }

/ProcessorPIIModes/ {
    if (section == "time") {
        line = $0
        # Extract benchmark name
        match(line, /ProcessorPIIModes\/[^[:space:]]+/)
        name = substr(line, RSTART, RLENGTH)
        
        split(name, parts, "/")

        if (length(parts) >= 3) {
            scenario = parts[2]
            mode = parts[3]

            # Extract the value after the benchmark name
            remainder = substr(line, RSTART + RLENGTH)
            gsub(/^[[:space:]]*/, "", remainder)

            # Extract the time value (handles both µs and ns)
            if (match(remainder, /[0-9]+\.?[0-9]*(µ|n)/)) {
                time_with_unit = substr(remainder, RSTART, RLENGTH)

                # Parse time value
                time_str = time_with_unit
                gsub(/[^0-9.]/, "", time_str)
                time_val = time_str + 0

                # Convert µs to ns
                if (index(time_with_unit, "µ") > 0) {
                    time_val = time_val * 1000
                }

                if (mode == "disabled-16") {
                    disabled_times[scenario] = time_val
                    disabled_count++
                    disabled_total += time_val
                } else if (mode == "enabled-16") {
                    enabled_times[scenario] = time_val
                    enabled_count++
                    enabled_total += time_val
                }
            }
        }
    }
}

END {
    # Calculate averages
    if (disabled_count > 0) disabled_avg = disabled_total / disabled_count
    if (enabled_count > 0) enabled_avg = enabled_total / enabled_count

    # Calculate overhead
    overhead_total = 0
    overhead_count = 0
    min_overhead = 999999
    max_overhead = 0
    min_scenario = ""
    max_scenario = ""

    for (scenario in disabled_times) {
        if (scenario in enabled_times) {
            overhead = enabled_times[scenario] / disabled_times[scenario]
            overhead_total += overhead
            overhead_count++

            if (overhead < min_overhead) {
                min_overhead = overhead
                min_scenario = scenario
            }
            if (overhead > max_overhead) {
                max_overhead = overhead
                max_scenario = scenario
            }
        }
    }

    if (overhead_count > 0) {
        avg_overhead = overhead_total / overhead_count

        printf "Average disabled time:      %.0f ns/op\n", disabled_avg
        printf "Average enabled time:       %.0f ns/op\n", enabled_avg
        printf "\n"
        printf "Enabled overhead (average): %.1fx slower\n", avg_overhead
        printf "Enabled overhead (minimum): %.1fx slower  [%s]\n", min_overhead, min_scenario
        printf "Enabled overhead (maximum): %.1fx slower  [%s]\n", max_overhead, max_scenario
        printf "\n"
        printf "Overall time increase:      %.1f%%\n", (enabled_avg / disabled_avg - 1) * 100
    } else {
        print "No data available for comparison"
    }
}
' "${TEMP_DIR}/benchstat.txt"

} | tee "${ANALYSIS_FILE}"

echo ""
echo "Analysis complete!"
echo "Results saved to:"
echo "  ${BENCHMARK_DIR}/benchstat_${TIMESTAMP}.txt"
echo "  ${ANALYSIS_FILE}"
echo ""
