#!/bin/sh
#
# System check for the dsd-alpine image. Builds and runs the image
# both in UDP and socket mode. With lsof, we test that dogstatsd is
# running and listening on the right interface
#
# Can be ported to Go once we add the docker go library to the build deps

DOCKER_IMAGE="dsd-alpine:system_test"
SOCKET_PATH="/statsd/statsd.socket"
DD_ARGS="-e DD_DD_URL=http://dummy -e DD_API_KEY=dummy -v `pwd`:/statsd:rw"
TEST_FAIL=0

if [ !$(which docker) ]; then
    echo "Installing docker (temporary workaround, will install in runner image)"
    curl -sL https://get.docker.com/builds/Linux/x86_64/docker-17.04.0-ce.tgz | tar xfz -
    chmod -R 0777 docker
    export PATH=$PWD/docker:$PATH
fi

echo "Building docker image:"

OUT=`cp bin/static/dogstatsd Dockerfiles/dogstatsd/alpine && \
docker build -t $DOCKER_IMAGE Dockerfiles/dogstatsd/alpine`

if [ $? -ne 0 ]; then
    echo "Error building the imagge"
    echo $OUT
    docker image rm $DOCKER_IMAGE > /dev/null
    exit 1
else
    echo "OK"
fi

# UDP_CO should listen on UDP 8125, but not on the socket

UDP_CO=`docker run --rm -d $DD_ARGS $DOCKER_IMAGE`

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

docker stop $UDP_CO > /dev/null

# SOCKET_CO should listen on the socket, but not on UDP 8125

SOCKET_CO=`docker run --rm -d -e DD_DOGSTATSD_SOCKET=$SOCKET_PATH $DD_ARGS $DOCKER_IMAGE`

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

docker stop $SOCKET_CO > /dev/null

if [ -f statsd.socket ]; then
    rm statsd.socket
fi

docker image rm $DOCKER_IMAGE > /dev/null

if [ $TEST_FAIL -eq 0 ]; then
    echo "Test succeeded"
else
    echo "Test failed"
fi

exit $TEST_FAIL
