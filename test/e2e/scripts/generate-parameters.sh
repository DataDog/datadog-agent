#!/bin/bash

##### A script to generate a unique namespace #####
##### and a parameters file for a workflow    #####


##### Exit on error #####
set -e

##### Functions #####

usage()
{
    echo '    Usage: ./generate_parameters.sh [[-w workflow -g workflow_group] | [-h]]
    Example: ./generate_parameters.sh -g workflow_group -w workflow
    Flags:
    -w, --workflow         workflow name
    -g, --workflow-group   workflow group name
    -o, --output-file      generated yaml file name (default parameters.yaml)
    -d, --workflows-dir    the directory where workflows are defined (default ../argo-workflows)'
}

check_yq_installed()
{
    if ! [ -x "$(command -v yq)" ]; then
        echo 'Error: yq is not installed.'
        exit 1
    fi
}

validate_input()
{
    # Validate workflow name characters
    if ! [[ $WORKFLOW =~ ^[0-9a-zA-Z-]+$ ]]; then
        echo "Error: Invalid workflow name format: $WORKFLOW"
        exit 1
    fi

    # Validate workflow group name characters
    if ! [[ $WORKFLOW_GROUP =~ ^[0-9a-zA-Z._-]+$ ]]; then
        echo "Error: Invalid workflow group name format: $WORKFLOW_GROUP"
        exit 1
    fi
}

generate_namespace()
{
    # Generate unique namespace
    # namespace format: <workflow_group>-<workflow>-<firs_5_chars_of_prefix_check_sum>-<random_5_digits>
    echo 'Info: Generating namespace...'
    PREFIX=$WORKFLOW_GROUP-$WORKFLOW
    # `_` and `.` are not allowed in namespace names, replace them with `-`
    PREFIX=${PREFIX//[_.]/-} 
    CHECK_SUM=$(echo -n $PREFIX | md5sum | cut -c1-5)
    SUFFIX=$RANDOM
    NAMESPACE=$PREFIX-$CHECK_SUM-$SUFFIX
    echo "Info: Generated namespace: $NAMESPACE"
}

generate_parameters()
{   
    # Merging parameters
    echo 'Info: Merging parameters...'
    YK_MERGE_COMMAND='yq merge --overwrite --allow-empty'
    DEFAULT_GLOBAL_PARAM="$WORKFLOWS_DIR/defaults/parameters.yaml"
    DEFAULT_GROUP_PARAM="$WORKFLOWS_DIR/$WORKFLOW_GROUP/defaults/parameters.yaml"
    WORKFLOW_PARAM="$WORKFLOWS_DIR/$WORKFLOW_GROUP/$WORKFLOW/parameters.yaml"
    TMP_YAML_PATH="$NAMESPACE.tmp.yaml"
    $YK_MERGE_COMMAND $DEFAULT_GLOBAL_PARAM $DEFAULT_GROUP_PARAM $WORKFLOW_PARAM > $TMP_YAML_PATH
    
    # Rendering namespace
    echo 'Info: Parameters merged, rendering namespace and saving file...'
    NAMESPACE_TEMPLATE_VAR="{{ namespace }}"
    sed -e "s/$NAMESPACE_TEMPLATE_VAR/$NAMESPACE/g" $TMP_YAML_PATH > $OUTPUT_YAML_FILE
    echo "Info: Generated parameters, yaml file saved: $OUTPUT_YAML_FILE"
    
    # Cleanup temp file
    rm $TMP_YAML_PATH
}


##### Main #####

WORKFLOW=""
WORKFLOW_GROUP=""
NAMESPACE=""
OUTPUT_YAML_FILE="parameters.yaml"
WORKFLOWS_DIR="../argo-workflows"

if [ "$1" == "" ]; then
    usage
    exit 1
fi

while [ "$1" != "" ]; do
    case $1 in
        -w | --workflow )       shift
                                WORKFLOW=$1
                                ;;
        -g | --workflow-group ) shift
                                WORKFLOW_GROUP=$1
                                ;;
        -o | --output-file )    shift
                                OUTPUT_YAML_FILE=$1
                                ;;
        -d | --workflows-dir )  shift
                                WORKFLOWS_DIR=$1
                                ;;
        -h | --help )           usage
                                exit
                                ;;
        * )                     usage
                                exit 1
    esac
    shift
done

# Only proceed when `yq` is installed
check_yq_installed

# Validate the parameters
validate_input

# Generate namespace
generate_namespace

# Generate the parameters file
generate_parameters
