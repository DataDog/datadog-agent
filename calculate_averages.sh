#!/bin/bash
# Script to calculate and add comprehensive statistics (avg, std dev, min, max, median) to timing file

TIMING_FILE="${1:-total_restart_timings.txt}"

if [ ! -f "$TIMING_FILE" ]; then
    echo "Error: File $TIMING_FILE not found"
    exit 1
fi

# Extract START and RESTART times, calculate statistics
# Uses awk to parse, then calculates stats including std dev, min, max, median

awk '
BEGIN {
    count = 0
}

/\[COMPARISON\] Total:/ {
    # Extract START time (format: START=XXXms or START=XXX.XXXms)
    if (match($0, /START=([0-9]+\.?[0-9]*)ms/, start_match)) {
        start_ms = start_match[1]
        start_times[count] = start_ms
    }
    
    # Extract RESTART time (format: RESTART=XXXms or RESTART=XXX.XXXms or RESTART=X.XXXs)
    if (match($0, /RESTART=([0-9]+\.?[0-9]*)(ms|s)/, restart_match)) {
        restart_val = restart_match[1]
        restart_unit = restart_match[2]
        
        if (restart_unit == "s") {
            restart_ms = restart_val * 1000
        } else {
            restart_ms = restart_val
        }
        restart_times[count] = restart_ms
        count++
    }
}

# Function to calculate standard deviation
function std_dev(values, avg, n) {
    if (n <= 1) return 0
    sum_sq_diff = 0
    for (i = 0; i < n; i++) {
        diff = values[i] - avg
        sum_sq_diff += diff * diff
    }
    variance = sum_sq_diff / (n - 1)
    return sqrt(variance)
}

# Function to calculate median
function median(values, n) {
    # Create a sorted copy
    for (i = 0; i < n; i++) {
        sorted[i] = values[i]
    }
    # Simple bubble sort (fine for small datasets)
    for (i = 0; i < n-1; i++) {
        for (j = 0; j < n-i-1; j++) {
            if (sorted[j] > sorted[j+1]) {
                temp = sorted[j]
                sorted[j] = sorted[j+1]
                sorted[j+1] = temp
            }
        }
    }
    # Return median
    if (n % 2 == 0) {
        return (sorted[n/2 - 1] + sorted[n/2]) / 2
    } else {
        return sorted[int(n/2)]
    }
}

END {
    if (count > 0) {
        # Calculate averages
        start_sum = 0
        restart_sum = 0
        start_min = start_times[0]
        start_max = start_times[0]
        restart_min = restart_times[0]
        restart_max = restart_times[0]
        
        for (i = 0; i < count; i++) {
            start_sum += start_times[i]
            restart_sum += restart_times[i]
            
            if (start_times[i] < start_min) start_min = start_times[i]
            if (start_times[i] > start_max) start_max = start_times[i]
            if (restart_times[i] < restart_min) restart_min = restart_times[i]
            if (restart_times[i] > restart_max) restart_max = restart_times[i]
        }
        
        avg_start = start_sum / count
        avg_restart = restart_sum / count
        
        # Calculate standard deviations
        start_std = std_dev(start_times, avg_start, count)
        restart_std = std_dev(restart_times, avg_restart, count)
        
        # Calculate medians
        start_median = median(start_times, count)
        restart_median = median(restart_times, count)
        
        # Print to file
        print "" >> "'"$TIMING_FILE"'"
        print "========================================" >> "'"$TIMING_FILE"'"
        print "SUMMARY STATISTICS" >> "'"$TIMING_FILE"'"
        print "========================================" >> "'"$TIMING_FILE"'"
        printf "Total runs: %d\n", count >> "'"$TIMING_FILE"'"
        print "" >> "'"$TIMING_FILE"'"
        print "START Times:" >> "'"$TIMING_FILE"'"
        printf "  Average:  %.3fms\n", avg_start >> "'"$TIMING_FILE"'"
        printf "  Median:   %.3fms\n", start_median >> "'"$TIMING_FILE"'"
        printf "  Std Dev:  %.3fms\n", start_std >> "'"$TIMING_FILE"'"
        printf "  Min:      %.3fms\n", start_min >> "'"$TIMING_FILE"'"
        printf "  Max:      %.3fms\n", start_max >> "'"$TIMING_FILE"'"
        printf "  Range:    %.3fms\n", start_max - start_min >> "'"$TIMING_FILE"'"
        print "" >> "'"$TIMING_FILE"'"
        print "RESTART Times:" >> "'"$TIMING_FILE"'"
        printf "  Average:  %.3fms\n", avg_restart >> "'"$TIMING_FILE"'"
        printf "  Median:   %.3fms\n", restart_median >> "'"$TIMING_FILE"'"
        printf "  Std Dev:  %.3fms\n", restart_std >> "'"$TIMING_FILE"'"
        printf "  Min:      %.3fms\n", restart_min >> "'"$TIMING_FILE"'"
        printf "  Max:      %.3fms\n", restart_max >> "'"$TIMING_FILE"'"
        printf "  Range:    %.3fms\n", restart_max - restart_min >> "'"$TIMING_FILE"'"
        print "" >> "'"$TIMING_FILE"'"
        print "Comparison:" >> "'"$TIMING_FILE"'"
        diff_ms = avg_restart - avg_start
        pct = (avg_restart / avg_start) * 100
        printf "  Average DIFF: %.3fms (RESTART is %.1f%% of START)\n", diff_ms, pct >> "'"$TIMING_FILE"'"
        print "========================================" >> "'"$TIMING_FILE"'"
        
        # Also print to stdout
        print ""
        print "========================================"
        print "SUMMARY STATISTICS"
        print "========================================"
        printf "Total runs: %d\n", count
        print ""
        print "START Times:"
        printf "  Average:  %.3fms\n", avg_start
        printf "  Median:   %.3fms\n", start_median
        printf "  Std Dev:  %.3fms\n", start_std
        printf "  Min:      %.3fms\n", start_min
        printf "  Max:      %.3fms\n", start_max
        printf "  Range:    %.3fms\n", start_max - start_min
        print ""
        print "RESTART Times:"
        printf "  Average:  %.3fms\n", avg_restart
        printf "  Median:   %.3fms\n", restart_median
        printf "  Std Dev:  %.3fms\n", restart_std
        printf "  Min:      %.3fms\n", restart_min
        printf "  Max:      %.3fms\n", restart_max
        printf "  Range:    %.3fms\n", restart_max - restart_min
        print ""
        print "Comparison:"
        printf "  Average DIFF: %.3fms (RESTART is %.1f%% of START)\n", diff_ms, pct
        print "========================================"
    } else {
        print "No timing data found in file"
    }
}
' "$TIMING_FILE"

