#!/bin/bash -l

# This script is meant to be run by someone editing the test kitchen. It runs kitchen diagnose.

# http://redsymbol.net/articles/unofficial-bash-strict-mode/
IFS=$'\n\t'
set -euxo pipefail

rm -rf .kitchen

if [ -f $(pwd)/ssh-key ]; then
  rm ssh-key
fi

ssh-keygen -f $(pwd)/ssh-key -P "" -t rsa -b 2048

export AZURE_SSH_KEY_PATH="$(pwd)/ssh-key"

if [ ! -f /root/.azure/credentials ]; then
  mkdir -p /root/.azure
  touch /root/.azure/credentials
fi

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

if [ -z ${AZURE_SUBSCRIPTION_ID+x} -o -z ${AZURE_TENANT_ID+x} -o -z ${AZURE_CLIENT_SECRET+x} -o -z ${AZURE_CLIENT_ID+x} ]; then
  printf "You are missing some of the necessary credentials. Exiting."
  exit 1
fi

(echo "<% subscription_id=\"$AZURE_SUBSCRIPTION_ID\"; client_id=\"$AZURE_CLIENT_ID\"; client_secret=\"$AZURE_CLIENT_SECRET\"; tenant_id=\"$AZURE_TENANT_ID\"; %>" && cat azure-creds.erb) | erb > /root/.azure/credentials
set -x


echo $(pwd)/ssh-key
echo $AZURE_SSH_KEY_PATH

eval $(ssh-agent -s)

ssh-add "$AZURE_SSH_KEY_PATH"

cp kitchen-azure.yml kitchen.yml
kitchen diagnose --no-instances --loader
