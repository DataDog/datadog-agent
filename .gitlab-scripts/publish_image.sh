#!/bin/bash

TAG=$1
cp ${@:2} Dockerfiles/agent
docker build -t stackstate/stackstate-agent:$TAG -t stackstate/stackstate-agent:latest Dockerfiles/agent
docker login -u $DOCKER_USER -p $DOCKER_PASS
docker push stackstate/stackstate-agent:$TAG
docker push stackstate/stackstate-agent:latest
