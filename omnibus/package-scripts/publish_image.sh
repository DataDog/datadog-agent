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

DOCKER_TAG="${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${IMAGE_TAG}"

docker login -u "${docker_user}" -p "${docker_password}" "${REGISTRY}"
docker build -t "${DOCKER_TAG}" "${DOCKERFILE_PATH}"
docker push "${DOCKER_TAG}"

if [ -n "$EXTRA_TAG" ]; then
    DOCKER_EXTRA_TAG="${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${EXTRA_TAG}"
    docker tag "${DOCKER_TAG}" "${DOCKER_EXTRA_TAG}"
    echo "Pushing release to ${EXTRA_TAG}"
    docker push "${DOCKER_EXTRA_TAG}"
fi

if [ ! -z "${CI_COMMIT_TAG}" ] || [ "${CI_COMMIT_BRANCH}" = "master" ]; then
    echo "Scanning image ${DOCKER_TAG} for vulnerabilities"
    ./anchore.sh -n 0 -i "${DOCKER_TAG}"
fi
