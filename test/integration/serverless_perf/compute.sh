#!/bin/bash

STARTUP_TIME_THREESHOLD=20
TOTAL_THREESHOLD=$((STARTUP_TIME_THREESHOLD*10))

totalMs=0

# loop 10 times to incur no false positive/negative alarms
for i in {1..10}
do
    # create a new container to ensure cold start
    dockerId=$(docker run -d datadogci/lambda-extension)
    sleep 10
    numberOfMillisecs=$(docker logs "$dockerId" | grep 'ready in' | grep -Eo '[0-9]{1,4}' | tail -3 | head -1)
    totalMs=$((totalMs+numberOfMillisecs))
    echo "Iteration $i - Statup time = $numberOfMillisecs"
done

echo "Total computed : $totalMs"
echo "Threshold : $TOTAL_THREESHOLD"

# check whether or not the total duration exceeds the threshold
if (( totalMs > TOTAL_THREESHOLD )); then
    exit 1
fi

exit 0