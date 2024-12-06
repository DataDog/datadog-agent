#!/bin/bash

set -e

echo "Building recorder extension"
cd recorder-extension

if [ -z "$ARCHITECTURE" ]; then
    echo "ARCHITECTURE is not set. Please set it to either 'amd64' or 'arm64'."
    exit 1
fi

BUILD_FILE=Dockerfile.build

function docker_build_zip {
    arch=$1

    DOCKER_BUILDKIT=1
    # This version number is arbitrary and won't be used by AWS
    VERSION=123

    echo "Building Docker image for $arch"
    docker buildx build --platform linux/${arch} \
        -t datadog/build-recorder-extension-${arch}:$VERSION \
        -f ./$BUILD_FILE \
        . --load

    echo "Creating Docker container to extract ZIP"
    dockerId=$(docker create datadog/build-recorder-extension-${arch}:$VERSION)

    echo "Copying ZIP file from Docker container"
    docker cp $dockerId:/recorder_extension.zip ./ext.zip

    # Clean up the Docker container
    docker rm $dockerId
}

if [ "$ARCHITECTURE" == "amd64" ]; then
    echo "Building for amd64"
    docker_build_zip amd64
elif [ "$ARCHITECTURE" == "arm64" ]; then
    echo "Building for arm64"
    docker_build_zip arm64
else
    echo "Invalid ARCHITECTURE param. Please set it to either 'amd64' or 'arm64'. Exiting."
    exit 1
fi
