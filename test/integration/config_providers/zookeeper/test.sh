#!/bin/bash

set -e

echo "Starting zookeeper server"
docker_id=`docker run -d -p 2181 zookeeper`
trap "docker rm -f $docker_id" SIGINT SIGTERM EXIT

ip=`docker inspect --format='{{(index (index .NetworkSettings.Ports "2181/tcp") 0).HostIp}}' $docker_id`
port=`docker inspect --format='{{(index (index .NetworkSettings.Ports "2181/tcp") 0).HostPort}}' $docker_id`

echo "Waiting for zookeeper to come up: $ip:$port"
res=""
i=0
while [[ $res == "" ]]; do
	res=`echo stat | nc $ip $port`
	i=$(($i+1))
	if [[ $i -gt 10 ]]; then
		echo "Timeout waiting for zookeeper to come up"
		exit 1
	fi
	sleep 1
done

echo "zookeeper up, starting tests"
ZK_URL="$ip:$port" go test -v .
exit $?
