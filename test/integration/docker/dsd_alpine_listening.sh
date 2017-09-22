#!/bin/bash

# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

# System check for the dsd-alpine image. Runs the image both in UDP and
# socket mode. With lsof, we test that dogstatsd is running and listening
# on the right interface


# $DOCKER_IMAGE has to be given as an environment variable
if [ -z $DOCKER_IMAGE ]; then
    echo "You must set the DOCKER_IMAGE environment variable to run the test"
    exit 1
fi

SOCKET_PATH="/tmp/statsd.socket"
DD_ARGS="-e DD_DD_URL=http://dummy -e DD_API_KEY=dummy"
TEST_FAIL=0

# Starting containers and waiting one second for dsd to listen (avoid flaky test)

UDP_CO=`docker run --rm -d $DD_ARGS $DOCKER_IMAGE`
SOCKET_CO=`docker run --rm -d -e DD_DOGSTATSD_SOCKET=$SOCKET_PATH -e DD_DOGSTATSD_PORT=0 $DD_ARGS $DOCKER_IMAGE`
BOTH_CO=`docker run --rm -d -e DD_DOGSTATSD_SOCKET=$SOCKET_PATH -e DD_DOGSTATSD_PORT=8125 $DD_ARGS $DOCKER_IMAGE`

sleep 1

# UDP_CO should listen on UDP 8125, but not on the socket

echo "Testing UDP container:"
docker exec $UDP_CO apk add --no-cache lsof > /dev/null

OUT=`docker exec $UDP_CO lsof -U | grep $SOCKET_PATH`
if [ $? -ne 1 ]; then
    TEST_FAIL=1
    echo "Error: listening on socket"
    echo $OUT
fi

OUT=`docker exec $UDP_CO lsof -i | grep "*:8125"`
if [ $? -ne 0 ]; then
    TEST_FAIL=1
    echo "Error: not listening on UDP"
    echo $OUT
fi

if [ $TEST_FAIL -eq 0 ]; then
    echo "OK"
fi

# SOCKET_CO should listen on the socket, but not on UDP 8125
# We don't bind the socket out of the container as we don't need to send anything

echo "Testing socket container:"
docker exec $SOCKET_CO apk add --no-cache lsof > /dev/null

OUT=`docker exec $SOCKET_CO lsof -U | grep $SOCKET_PATH`
if [ $? -ne 0 ]; then
    TEST_FAIL=1
    echo "Error: not listening on socket"
    echo $OUT
fi

OUT=`docker exec $SOCKET_CO lsof -i | grep 8125`
if [ $? -ne 1 ]; then
    TEST_FAIL=1
    echo "Error: listening on UDP"
    echo $OUT
fi

if [ $TEST_FAIL -eq 0 ]; then
    echo "OK"
fi

# BOTH_CO should listento both the socket and UDP 8125

echo "Testing udp+socket container:"
docker exec $BOTH_CO apk add --no-cache lsof > /dev/null

OUT=`docker exec $BOTH_CO lsof -U | grep $SOCKET_PATH`
if [ $? -ne 0 ]; then
    TEST_FAIL=1
    echo "Error: not listening on socket"
    echo $OUT
fi

OUT=`docker exec $BOTH_CO lsof -i | grep 8125`
if [ $? -ne 0 ]; then
    TEST_FAIL=1
    echo "Error: not listening on UDP"
    echo $OUT
fi

if [ $TEST_FAIL -eq 0 ]; then
    echo "OK"
fi

# Cleanup

docker stop $UDP_CO $SOCKET_CO $BOTH_CO > /dev/null

# Conclusion

if [ $TEST_FAIL -eq 0 ]; then
    echo "Test succeeded"
else
    echo "Test failed"
fi

exit $TEST_FAIL
