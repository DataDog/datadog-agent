#!/bin/bash -l

# This script lists any stray azure vms that remain from an arbitrary pipeline.
# It's meant to be run manually to see if any cleanup is necessary.

# http://redsymbol.net/articles/unofficial-bash-strict-mode/
IFS=$'\n\t'
set -euxo pipefail

# These should not be printed out
set +x
if [ -z ${AZURE_CLIENT_ID+x} ]; then
  export AZURE_CLIENT_ID=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_client_id --with-decryption --query "Parameter.Value" --out text)
fi
if [ -z ${AZURE_CLIENT_SECRET+x} ]; then
  export AZURE_CLIENT_SECRET=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_client_secret --with-decryption --query "Parameter.Value" --out text)
fi
if [ -z ${AZURE_TENANT_ID+x} ]; then
  export AZURE_TENANT_ID=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_tenant_id --with-decryption --query "Parameter.Value" --out text)
fi
if [ -z ${AZURE_SUBSCRIPTION_ID+x} ]; then
  export AZURE_SUBSCRIPTION_ID=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_subscription_id --with-decryption --query "Parameter.Value" --out text)
fi
if [ -z ${CI_PIPELINE_ID+x} ]; then
  export CI_PIPELINE_ID='none'
fi

if [ -z ${AZURE_SUBSCRIPTION_ID+x} -o -z ${AZURE_TENANT_ID+x} -o -z ${AZURE_CLIENT_SECRET+x} -o -z ${AZURE_CLIENT_ID+x} ]; then
  printf "You are missing some of the necessary credentials. Exiting."
  exit 1
fi

az login --service-principal -u $AZURE_CLIENT_ID -p $AZURE_CLIENT_SECRET --tenant $AZURE_TENANT_ID > /dev/null
set -x

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
