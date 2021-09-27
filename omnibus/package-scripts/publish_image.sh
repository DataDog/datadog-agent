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
    DOCKER_TAG="${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${IMAGE_TAG}"

    docker tag "${BUILD_TAG}" "${DOCKER_TAG}"
    docker push "${DOCKER_TAG}"

    if [ -n "$EXTRA_TAG" ]; then
        DOCKER_EXTRA_TAG="${REGISTRY}/${ORGANIZATION}/${IMAGE_REPO}:${EXTRA_TAG}"
        docker tag "${DOCKER_TAG}" "${DOCKER_EXTRA_TAG}"
        echo "Pushing release to ${EXTRA_TAG}"
        docker push "${DOCKER_EXTRA_TAG}"
    fi
done

if [ ! -z "${CI_COMMIT_TAG}" ] || [ "${CI_COMMIT_BRANCH}" = "master" ]; then
    # for Anchore use publicly accessible image tag
    DOCKER_TAG="${REGISTRY}/${ORGANIZATION}/${REGISTRY_DOCKERHUB}:${IMAGE_TAG}"
    echo "Scanning image ${DOCKER_TAG} for vulnerabilities"
    ./anchore.sh -n 0 -i "${DOCKER_TAG}"
fi
