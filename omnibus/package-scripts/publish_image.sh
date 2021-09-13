#!/bin/sh

set -xe

IMAGE_TAG="${1}"
IMAGE_REPO="${2}"
DOCKERFILE_PATH="${3}"
EXTRA_TAG="${4}"
REGISTRY_DOCKERHUB="docker.io"
REGISTRY_QUAY="quay.io"
ORGANIZATION="stackstate"

echo "IMAGE_TAG=${IMAGE_TAG}"
echo "IMAGE_REPO=${IMAGE_REPO}"
echo "DOCKERFILE_PATH=${DOCKERFILE_PATH}"

BUILD_TAG="${IMAGE_REPO}:${IMAGE_TAG}"

docker login -u "${docker_user}" -p "${docker_password}" "${REGISTRY}"
docker login -u "${quay_user}" -p "${quay_password}" "${REGISTRY_QUAY}"

docker build -t "${BUILD_TAG}" "${DOCKERFILE_PATH}"

for REGISTRY in "${REGISTRY_DOCKERHUB}" "${REGISTRY_QUAY}"; do
    docker tag "${BUILD_TAG}" "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${IMAGE_TAG}"
    docker push "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${IMAGE_TAG}"

    if [ -n "$EXTRA_TAG" ]; then
        docker tag "${BUILD_TAG}" "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${EXTRA_TAG}"
        echo "Pushing release to ${EXTRA_TAG}"

        docker push "${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${EXTRA_TAG}"
    fi
done
