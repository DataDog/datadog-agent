#!/bin/sh

set -xe

IMAGE_TAG="${1}"
IMAGE_REPO="${2}"
DOCKERFILE_PATH="${3}"
EXTRA_TAG="${4}"
REGISTRY="${5:-docker.io}"
ORGANIZATION="${6:-stackstate}"

echo "${IMAGE_TAG}"
echo "${IMAGE_REPO}"
echo "${DOCKERFILE_PATH}"

docker login -u "${docker_user}" -p "${docker_password}" "${REGISTRY}"
docker build -t "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${IMAGE_TAG}" "${DOCKERFILE_PATH}"
docker push "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${IMAGE_TAG}"

if [ -n "$EXTRA_TAG" ]; then
    docker tag "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${IMAGE_TAG}" "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${EXTRA_TAG}"
    echo "Pushing release to ${EXTRA_TAG}"
    docker push "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${EXTRA_TAG}"
fi
