#!/bin/sh

set -xe

IMAGE_TAG="${1}"
IMAGE_REPO="${2}"
DOCKERFILE_PATH="${3}"
PUSH_LATEST="${4:-false}"
REGISTRY="${5:-docker.io}"
ORGANIZATION="${6:-stackstate}"

echo "${IMAGE_TAG}"
echo "${IMAGE_REPO}"
echo "${DOCKERFILE_PATH}"

docker build -t "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${IMAGE_TAG}" "${DOCKERFILE_PATH}"
docker login -u "${docker_user}" -p "${docker_password}" "${REGISTRY}"
docker push "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${IMAGE_TAG}"

if [ "$PUSH_LATEST" = "true" ]; then
    docker tag "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${IMAGE_TAG}" "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:latest"
    echo 'Pushing release to latest'
    docker push "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:latest"
fi
