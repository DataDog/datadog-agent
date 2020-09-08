#!/bin/bash

# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2020 Datadog, Inc.

# Publish the datadog lambda layer across regions, using the AWS CLI
# Usage: publish_layer.sh [region] [layer]
# Specifying the region and layer arg will publish the specified layer to the specified region
set -e

# Makes sure any subprocesses will be terminated with this process
trap "pkill -P $$; exit 1;" INT

# Move into the root directory, so this script can be called from any directory
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
cd $DIR/../..

LAYER_PATH=".layers/datadog_extension.zip"
# aws-cli supports a maximum of 5 compatible runtimes per layer, so we group by language
VERSIONS_FOR_AWS_CLI=("python2.7 python3.6 python3.7 python3.8" "nodejs10.x nodejs12.x" "java11 java8.al2 java8" "ruby2.7 ruby2.5" "dotnetcore2.1 dotnetcore3.1" "provided.al2 provided")
AVAILABLE_LAYER_NAMES=("Datadog-Extension-Python" "Datadog-Extension-Node" "Datadog-Extension-Java" "Datadog-Extension-Ruby" "Datadog-Extension-DotNet" "Datadog-Extension-Provided")
AVAILABLE_REGIONS=$(aws ec2 describe-regions | jq -r '.[] | .[] | .RegionName')

# Check that the layer files exist

if [ ! -f $LAYER_PATH  ]; then
    echo "Could not find $LAYER_PATH."
    exit 1
fi


# Check region arg
if [ -z "$1" ]; then
    echo "Region parameter not specified, running for all available regions."
    REGIONS=$AVAILABLE_REGIONS
else
    echo "Region parameter specified: $1"
    if [[ ! "$AVAILABLE_REGIONS" == *"$1"* ]]; then
        echo "Could not find $1 in available regions: $AVAILABLE_REGIONS"
        echo ""
        echo "EXITING SCRIPT."
        exit 1
    fi
    REGIONS=($1)
fi

echo "Starting publishing layers for regions: $REGIONS"

# Check layer_name arg
if [ -z "$2" ]; then
    echo "Layer name parameter not specified, running for all layer names."
    LAYER_NAMES=("${AVAILABLE_LAYER_NAMES[@]}")
else
    echo "Layer name parameter specified: $2"
    if [[ ! " ${AVAILABLE_LAYER_NAMES[@]} " =~ " ${2} " ]]; then
        echo "Could not find $2 in available layer names: ${AVAILABLE_LAYER_NAMES[@]}"
        echo ""
        echo "EXITING SCRIPT."
        exit 1
    fi
    LAYER_NAMES=($2)
fi




echo "Publishing layers: ${LAYER_NAMES[*]}"

publish_layer() {
    region=$1
    layer_name=$2
    aws_version_key=$3
    version_nbr=$(aws lambda publish-layer-version --layer-name $layer_name \
        --description "Datadog Lambda Extension" \
        --zip-file "fileb://$LAYER_PATH" \
        --region $region \
        --compatible-runtimes $aws_version_key \
                        | jq -r '.Version')

    aws lambda add-layer-version-permission --layer-name $layer_name \
        --version-number $version_nbr \
        --statement-id "release-$version_nbr" \
        --action lambda:GetLayerVersion --principal "*" \
        --region $region

    echo "Published layer for region $region, version $aws_version_key, layer_name $layer_name, layer_version $version_nbr"
}

BATCH_SIZE=1
PIDS=()

wait_for_processes() {
    for pid in "${PIDS[@]}"; do
        wait $pid
    done
    PIDS=()
}

for region in $REGIONS
do
    echo "Starting publishing layer for region $region..."

    # Publish the layers for each runtime type
    i=0
    for layer_name in "${LAYER_NAMES[@]}"; do
        aws_version_key="${VERSIONS_FOR_AWS_CLI[$i]}"

        publish_layer $region $layer_name $aws_version_key &
        PIDS+=($!)
        if [ ${#PIDS[@]} -eq $BATCH_SIZE ]; then
            wait_for_processes
        fi

        i=$(expr $i + 1)

    done
done

wait_for_processes

echo "Done !"
