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
    echo "Pipeline Id=$CI_PIPELINE_ID | Job Id=$CI_JOB_ID | $1"
}

startupTimes=()

# loop 10 times to incur no false positive/negative alarms
for i in {1..10}
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

# check whether or not the median duration exceeds the threshold
if (( medianMs > STARTUP_TIME_THRESHOLD )); then
    exit 1
fi

exit 0
