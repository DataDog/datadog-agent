#!/bin/sh

set -xe

IMAGE_TAG=$1
IMAGE_REPO=$2
ARTIFACT_PATH=$3

echo $IMAGE_TAG
echo $IMAGE_REPO
echo $ARTIFACT_PATH

cp $ARTIFACT_PATH/*.deb Dockerfiles/agent

docker build -t stackstate/${IMAGE_REPO}:${IMAGE_TAG} Dockerfiles/agent
docker tag stackstate/${IMAGE_REPO}:${IMAGE_TAG} stackstate/${IMAGE_REPO}:latest

docker login -u $DOCKER_USER -p $DOCKER_PASS
docker push stackstate/${IMAGE_REPO}:${IMAGE_TAG}
