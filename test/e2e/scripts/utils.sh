#!/bin/bash

##### A script to generate a unique namespace #####
##### and a parameters file for a workflow    #####


##### Exit on error #####
set -e

##### Utility functions #####

# Usage: generate_namespace <workflow_group> <workflow>
generate_namespace()
{
    # Generate unique namespace from workflow_group and workflow
    # namespace format: <workflow_group>-<workflow>-<firs_5_chars_of_prefix_check_sum>-<random_5_digits>
    echo 'Info: Generating namespace...'
    PREFIX=$1-$2
    # `_` and `.` are not allowed in namespace names, replace them with `-`
    PREFIX=${PREFIX//[_.]/-} 
    CHECK_SUM=$(echo -n "$PREFIX" | md5sum | cut -c1-15)
    NAMESPACE=$PREFIX-$CHECK_SUM
    if ! [[ $NAMESPACE =~ ^[0-9a-zA-Z-]+$ ]]; then
        echo "Error: Invalid namespace format: $NAMESPACE"
        exit 1
    fi
    echo "Info: Generated namespace: $NAMESPACE"
}

# Usage: check_yq_installed
check_yq_installed()
{
    if ! [ -x "$(command -v yq)" ]; then
        echo 'Error: yq is not installed.'
        exit 1
    fi
}