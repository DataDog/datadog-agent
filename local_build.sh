#!/bin/bash

# This script sets up a local docker container for building the agent. It will mount
# the current directory ready only, and make a clone inside docker, which can be build against. A typical workflow
# would be:

#  > ./local_build.sh
# < make changes to host repo and commit ?
# < pull changes in the docker shell >
# < rebuild in the docker shell >

set -euxo pipefail

NAME=agent_local_build

if [[ "$#" -eq "1" ]]; then
    if [[ "$1" =  "restart" ]]; then
        ( cd Dockerfiles/local_builder && docker build -t agent_build . )

        docker rm -f agent_local_build || true

        echo "This docker file will setup a docker container with a clone of the current agent directory."
        echo "The current directory will be mounted as a volume and can be pulled from, but the build is fully separated form the host system"

        CURBRANCH=`git rev-parse --abbrev-ref HEAD`
        MOUNT="/stackstate-agent-mount"

        docker run --rm \
            -e ARTIFACTORY_USER=$ARTIFACTORY_USER \
            -e ARTIFACTORY_PASSWORD=$ARTIFACTORY_PASSWORD \
            -e ARTIFACTORY_URL="artifactory.stackstate.io/artifactory/api/pypi/pypi-local" \
            -it --name $NAME -v "`pwd`:$MOUNT:ro" agent_build:latest "$MOUNT" $CURBRANCH

        exit 0
    fi
fi

echo "Attaching to current container, use <restart> to setup again"

docker exec -it $NAME bash --init-file /shell.sh
