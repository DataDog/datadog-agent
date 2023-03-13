#!/bin/bash

# This script cleans up any stray azure vms that may remain from the prior run.

# http://redsymbol.net/articles/unofficial-bash-strict-mode/
IFS=$'\n\t'
set -euxo pipefail

# These should not be printed out
set +x
if [ -z ${AZURE_CLIENT_ID+x} ]; then
  AZURE_CLIENT_ID=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_kitchen_client_id --with-decryption --query "Parameter.Value" --out text)
  export AZURE_CLIENT_ID
fi
if [ -z ${AZURE_CLIENT_SECRET+x} ]; then
  AZURE_CLIENT_SECRET=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_kitchen_client_secret --with-decryption --query "Parameter.Value" --out text)
  export AZURE_CLIENT_SECRET
fi
if [ -z ${AZURE_TENANT_ID+x} ]; then
  AZURE_TENANT_ID=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_kitchen_tenant_id --with-decryption --query "Parameter.Value" --out text)
  export AZURE_TENANT_ID
fi
if [ -z ${AZURE_SUBSCRIPTION_ID+x} ]; then
  AZURE_SUBSCRIPTION_ID=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_kitchen_subscription_id --with-decryption --query "Parameter.Value" --out text)
  export AZURE_SUBSCRIPTION_ID
fi
if [ -z ${DD_PIPELINE_ID+x} ]; then
  DD_PIPELINE_ID='none'
  export DD_PIPELINE_ID
fi

if [ -z ${AZURE_SUBSCRIPTION_ID+x} ] || [ -z ${AZURE_TENANT_ID+x} ] || [ -z ${AZURE_CLIENT_SECRET+x} ] || [ -z ${AZURE_CLIENT_ID+x} ]; then
  printf "You are missing some of the necessary credentials. Exiting."
  exit 1
fi

az login --service-principal -u "$AZURE_CLIENT_ID" -p "$AZURE_CLIENT_SECRET" --tenant "$AZURE_TENANT_ID" > /dev/null
set -x

if [ ${CLEAN_ALL+x} ]; then
  groups=$(az group list -o tsv --query "[?starts_with(name, 'kitchen')].[name]")
else
  groups=$(az group list -o tsv --query "[?starts_with(name, 'kitchen')]|[?ends_with(name, 'pl$DD_PIPELINE_ID')].[name]")
fi

# This will really only fail if a VM or Group
# is in the process of being deleted when queried but is deleted
# when the deletion attempt is made.
# So, failure should generally be swallowed.

for group in $groups; do
  echo "az group delete -n $group -y"
  if [ ${CLEAN_ASYNC+x} ]; then
    ( az group delete -n "$group" -y || true ) &
  else
    ( az group delete -n "$group" -y || true )
  fi
  printf "\n\n"
done

if [ ${CLEAN_ALL+x} ]; then
  vms=$(az vm list --query "[?tags.dd_agent_testing=='dd_agent_testing']|[*].[id]" -o tsv)
else
  vms=$(az vm list --query "[?tags.dd_agent_testing=='dd_agent_testing']|[?tags.pipeline_id=='$DD_PIPELINE_ID']|[*].[id]" -o tsv)
fi

for vm in $vms; do
  echo "az vm delete --ids $vm -y"
  if [ ${CLEAN_ASYNC+x} ]; then
    (az vm delete --ids "$vm" -y || true) &
  else
    (az vm delete --ids "$vm" -y || true)
  fi
done
