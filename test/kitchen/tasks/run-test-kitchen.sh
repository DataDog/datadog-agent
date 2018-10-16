#!/bin/bash -l

# This script sets up the environment and then runs the test kitchen itself.

# http://redsymbol.net/articles/unofficial-bash-strict-mode/
IFS=$'\n\t'
set -euxo pipefail

rm -rf .kitchen/logs/*

# Ensure that the ssh key is never reused between tests
if [ -f $(pwd)/ssh-key ]; then
  rm ssh-key
fi
if [ -f $(pwd)/ssh-key.pub ]; then
  rm ssh-key.pub
fi

ssh-keygen -f $(pwd)/ssh-key -P "" -t rsa -b 2048
export AZURE_SSH_KEY_PATH="$(pwd)/ssh-key"

# show that the ssh key is there
echo $(pwd)/ssh-key
echo $AZURE_SSH_KEY_PATH

# start the ssh-agent and add the key
eval $(ssh-agent -s)
ssh-add "$AZURE_SSH_KEY_PATH"

# in docker we cannot interact to do this so we must disable it
mkdir -p ~/.ssh
[[ -f /.dockerenv ]] && echo -e "Host *\n\tStrictHostKeyChecking no\n\n" > ~/.ssh/config

# load chef into the environment
eval "$(chef shell-init bash)"

# Setup the azure credentials, grabbing them from AWS if they do not exist in the environment already
# If running locally, they should be imported into the environment
if [ ! -f /root/.azure/credentials ]; then
  mkdir -p /root/.azure
  touch /root/.azure/credentials
fi

# These should not be printed out
set +x
if [ -z ${AZURE_CLIENT_ID+x} ]; then
  AZURE_CLIENT_ID=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_client_id --with-decryption --query "Parameter.Value" --out text)
  # make sure whitespace is removed
  export AZURE_CLIENT_ID="$(echo -e "${AZURE_CLIENT_ID}" | tr -d '[:space:]')"
fi
if [ -z ${AZURE_CLIENT_SECRET+x} ]; then
  AZURE_CLIENT_SECRET=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_client_secret --with-decryption --query "Parameter.Value" --out text)
  # make sure whitespace is removed
  export AZURE_CLIENT_SECRET="$(echo -e "${AZURE_CLIENT_SECRET}" | tr -d '[:space:]')"
fi
if [ -z ${AZURE_TENANT_ID+x} ]; then
  AZURE_TENANT_ID=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_tenant_id --with-decryption --query "Parameter.Value" --out text)
  # make sure whitespace is removed
  export AZURE_TENANT_ID="$(echo -e "${AZURE_TENANT_ID}" | tr -d '[:space:]')"
fi
if [ -z ${AZURE_SUBSCRIPTION_ID+x} ]; then
  AZURE_SUBSCRIPTION_ID=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_subscription_id --with-decryption --query "Parameter.Value" --out text)
  # make sure whitespace is removed
  export AZURE_SUBSCRIPTION_ID="$(echo -e "${AZURE_SUBSCRIPTION_ID}" | tr -d '[:space:]')"
fi

if [ -z ${AZURE_SUBSCRIPTION_ID+x} -o -z ${AZURE_TENANT_ID+x} -o -z ${AZURE_CLIENT_SECRET+x} -o -z ${AZURE_CLIENT_ID+x} ]; then
  printf "You are missing some of the necessary credentials. Exiting."
  exit 1
fi

# Create the Azure credentials file
(echo "<% subscription_id=\"$AZURE_SUBSCRIPTION_ID\"; client_id=\"$AZURE_CLIENT_ID\"; client_secret=\"$AZURE_CLIENT_SECRET\"; tenant_id=\"$AZURE_TENANT_ID\"; %>" && cat azure-creds.erb) | erb > /root/.azure/credentials
set -x

# if the agent version isn't set, grab it
# This is for the windows agent, as it needs to know the exact right version to grab
# on linux it can just download the latest version from the package manager
if [ -z ${AGENT_VERSION+x} ]; then
  pushd ../..
    export AGENT_VERSION=`inv version --url-safe --git-sha-length=9`
  popd
fi

# Generate a password to use for the windows servers
if [ -z ${SERVER_PASSWORD+x} ]; then
  export SERVER_PASSWORD=$(< /dev/urandom tr -dc A-Za-z0-9 | head -c32)
fi

chef gem install net-ssh berkshelf rake psych:2.2.2 kitchen-azurerm:0.13.0 test-kitchen
cp .kitchen-azure.yml .kitchen.yml
chef exec kitchen diagnose --no-instances --loader

rm -rf cookbooks
rm -f Berksfile.lock

chef exec rake dd-agent-azure-parallel[20]
