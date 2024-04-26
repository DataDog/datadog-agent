#!/bin/bash

# This script lists any stray azure vms that remain from an arbitrary pipeline.
# It's meant to be run manually to see if any cleanup is necessary.

# http://redsymbol.net/articles/unofficial-bash-strict-mode/
IFS=$'\n\t'
set -euo pipefail

# These should not be printed out
set +x
if [ -z ${AZURE_CLIENT_ID+x} ]; then
  AZURE_CLIENT_ID=$($CI_PROJECT_DIR/tools/ci/aws_ssm_get_wrapper.sh $KITCHEN_AZURE_CLIENT_ID_SSM_NAME)
  export AZURE_CLIENT_ID
fi
if [ -z ${AZURE_CLIENT_SECRET+x} ]; then
  AZURE_CLIENT_SECRET=$($CI_PROJECT_DIR/tools/ci/aws_ssm_get_wrapper.sh $KITCHEN_AZURE_CLIENT_SECRET_SSM_NAME)
  export AZURE_CLIENT_SECRET
fi
if [ -z ${AZURE_TENANT_ID+x} ]; then
  AZURE_TENANT_ID=$($CI_PROJECT_DIR/tools/ci/aws_ssm_get_wrapper.sh $KITCHEN_AZURE_TENANT_ID_SSM_NAME)
  export AZURE_TENANT_ID
fi
if [ -z ${AZURE_SUBSCRIPTION_ID+x} ]; then
  AZURE_SUBSCRIPTION_ID=$($CI_PROJECT_DIR/tools/ci/aws_ssm_get_wrapper.sh $KITCHEN_AZURE_SUBSCRIPTION_ID_SSM_NAME)
  export AZURE_SUBSCRIPTION_ID
fi
if [ -z ${DD_PIPELINE_ID+x} ]; then
  DD_PIPELINE_ID='none'
  export DD_PIPELINE_ID
fi

if [ -z ${AZURE_SUBSCRIPTION_ID+x} -o -z ${AZURE_TENANT_ID+x} -o -z ${AZURE_CLIENT_SECRET+x} -o -z ${AZURE_CLIENT_ID+x} ]; then
  printf "You are missing some of the necessary credentials. Exiting."
  exit 1
fi

az login --service-principal -u "$AZURE_CLIENT_ID" -p "$AZURE_CLIENT_SECRET" --tenant "$AZURE_TENANT_ID" > /dev/null

printf "VMs:\n"

if [ ${SHOW_ALL+x} ]; then
  export VM_QUERY="[?tags.dd_agent_testing=='dd_agent_testing']"
else
  export VM_QUERY="[?tags.dd_agent_testing=='dd_agent_testing']|[?tags.pipeline_id=='$CI_PIPELINE_ID']"
fi

if [ ${STRAYS_VERBOSE+x} ]; then
  az vm list --query "$VM_QUERY"
else
  az vm list --query "$VM_QUERY|[*].{name:name,location:location,state:provisioningState}" -o tsv
fi

printf "\n"

printf "Groups:\n"

if [ ${SHOW_ALL+x} ]; then
  export GROUPS_QUERY="[?starts_with(name, 'kitchen-')]"
else
  export GROUPS_QUERY="[?starts_with(name, 'kitchen-')]|[?ends_with(name, 'pl"$CI_PIPELINE_ID"')]"
fi
if [ ${STRAYS_VERBOSE+x} ]; then
  az group list --query "$GROUPS_QUERY"
else
  az group list --query "$GROUPS_QUERY|[*].{name:name,location:location,state:properties.provisioningState}" -o table
fi
