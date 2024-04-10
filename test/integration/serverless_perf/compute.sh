#!/bin/bash

STARTUP_TIME_THRESHOLD=20

calculate_median() {
    local sorted=($(printf "%s\n" "${@}" | sort -n))

    local len=${#sorted[@]}
    local middle=$((len / 2))

    if ((len % 2 == 0)); then
        local median=$(((${sorted[middle - 1]} + ${sorted[middle]}) / 2))
    else
        local median=${sorted[middle]}
    fi

    echo "$median"
}

log() {
    echo "Job Name=$CI_JOB_NAME | Pipeline Id=$CI_PIPELINE_ID | Job Id=$CI_JOB_ID | Branch Name=$CI_COMMIT_BRANCH | $1"
}

startupTimes=()

# Cold start container enough times to get a reasonable histogram in <= 1 hour.
# Pick an odd number to avoid having to average two elements. Larger iteration
# counts are useful for exploiting large-sample-size statistics.
ITERATION_COUNT=301

for i in $(seq 1 ${ITERATION_COUNT})
do
    # create a new container to ensure cold start
    dockerId=$(docker run -d datadogci/lambda-extension)
    sleep 10
    numberOfMillisecs=$(docker logs "$dockerId" | grep 'ready in' | grep -Eo '[0-9]{1,4}' | tail -3 | head -1)
    startupTimes+=($numberOfMillisecs)
    log "Iteration=$i | Startup Time=$numberOfMillisecs"
done

medianMs=$(calculate_median "${startupTimes[@]}")

log "Median=$medianMs | Threshold=$STARTUP_TIME_THRESHOLD"

# Log raw startup time data in a CSV-like format for plotting a histogram
# & exploratory data analysis.
IFS=$',' log "RawData=${startupTimes[*]}"

# check whether or not the median duration exceeds the threshold
if (( medianMs > STARTUP_TIME_THRESHOLD )); then
    echo "Median startup time above threshold"
    exit 1
fi

exit 0
