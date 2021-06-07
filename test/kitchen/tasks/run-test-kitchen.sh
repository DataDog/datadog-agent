#!/bin/bash -l

# This script sets up the environment and then runs the test kitchen itself.

# http://redsymbol.net/articles/unofficial-bash-strict-mode/
IFS=$'\n\t'
set -euxo pipefail

# Ensure that the ssh key is never reused between tests
if [ -f "$(pwd)/ssh-key" ]; then
  rm ssh-key
fi
if [ -f "$(pwd)/ssh-key.pub" ]; then
  rm ssh-key.pub
fi

ssh-keygen -f "$(pwd)/ssh-key" -P "" -t rsa -b 2048
KITCHEN_SSH_KEY_PATH="$(pwd)/ssh-key"
export KITCHEN_SSH_KEY_PATH

# show that the ssh key is there
echo "$(pwd)/ssh-key"
echo "$KITCHEN_SSH_KEY_PATH"

# start the ssh-agent and add the key
eval "$(ssh-agent -s)"
ssh-add "$KITCHEN_SSH_KEY_PATH"

# in docker we cannot interact to do this so we must disable it
mkdir -p ~/.ssh
[[ -f /.dockerenv ]] && echo -e "Host *\n\tStrictHostKeyChecking no\n\n" > ~/.ssh/config

if [ "$KITCHEN_PROVIDER" == "azure" ]; then
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
    AZURE_CLIENT_ID="$(echo -e "${AZURE_CLIENT_ID}" | tr -d '[:space:]')"
    export AZURE_CLIENT_ID
  fi
  if [ -z ${AZURE_CLIENT_SECRET+x} ]; then
    AZURE_CLIENT_SECRET=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_client_secret --with-decryption --query "Parameter.Value" --out text)
    # make sure whitespace is removed
    AZURE_CLIENT_SECRET="$(echo -e "${AZURE_CLIENT_SECRET}" | tr -d '[:space:]')"
    export AZURE_CLIENT_SECRET
  fi
  if [ -z ${AZURE_TENANT_ID+x} ]; then
    AZURE_TENANT_ID=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_tenant_id --with-decryption --query "Parameter.Value" --out text)
    # make sure whitespace is removed
    AZURE_TENANT_ID="$(echo -e "${AZURE_TENANT_ID}" | tr -d '[:space:]')"
    export AZURE_TENANT_ID
  fi
  if [ -z ${AZURE_SUBSCRIPTION_ID+x} ]; then
    AZURE_SUBSCRIPTION_ID=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.azure_subscription_id --with-decryption --query "Parameter.Value" --out text)
    # make sure whitespace is removed
    AZURE_SUBSCRIPTION_ID="$(echo -e "${AZURE_SUBSCRIPTION_ID}" | tr -d '[:space:]')"
    export AZURE_SUBSCRIPTION_ID
  fi

  if [ -z ${AZURE_SUBSCRIPTION_ID+x} ] || [ -z ${AZURE_TENANT_ID+x} ] || [ -z ${AZURE_CLIENT_SECRET+x} ] || [ -z ${AZURE_CLIENT_ID+x} ]; then
    printf "You are missing some of the necessary credentials. Exiting."
    exit 1
  fi

  # Create the Azure credentials file
  (echo "<% subscription_id=\"$AZURE_SUBSCRIPTION_ID\"; client_id=\"$AZURE_CLIENT_ID\"; client_secret=\"$AZURE_CLIENT_SECRET\"; tenant_id=\"$AZURE_TENANT_ID\"; %>" && cat azure-creds.erb) | erb > /root/.azure/credentials
  set -x

elif [ "$KITCHEN_PROVIDER" == "ec2" ]; then
  echo "using ec2 kitchen provider"
fi

# Generate a password to use for the windows servers
if [ -z ${SERVER_PASSWORD+x} ]; then
  export SERVER_PASSWORD="$(< /dev/urandom tr -dc A-Za-z0-9 | head -c32)0aZ"
fi

if [[ $# == 0 ]]; then
  echo "Missing run suite argument. Exiting."
  exit 1
fi

if [[ $# == 1 ]]; then
  echo "Missing major version argument. Exiting."
  exit 1
fi

export MAJOR_VERSION=$2

# if the agent version isn't set, grab it
# This is for the windows agent, as it needs to know the exact right version to grab
# on linux it can just download the latest version from the package manager
if [ -z ${AGENT_VERSION+x} ]; then
  pushd ../..
    AGENT_VERSION=$(inv agent.version --url-safe --git-sha-length=7 --major-version "$MAJOR_VERSION")
    export AGENT_VERSION
    DD_AGENT_EXPECTED_VERSION=$(inv agent.version --url-safe --git-sha-length=7 --major-version "$MAJOR_VERSION")
    export DD_AGENT_EXPECTED_VERSION
  popd
fi

invoke -e kitchen.genconfig --platform="$KITCHEN_PLATFORM" --osversions="$KITCHEN_OSVERS" --provider="$KITCHEN_PROVIDER" --arch="${KITCHEN_ARCH:-x86_64}" --testfiles="$1" ${KITCHEN_FIPS:+--fips}

bundle exec kitchen diagnose --no-instances --loader

## copy the generated kitchen.yml to the .kitchen directory so it'll be included
## in the artifacts (for debugging when necessary)
cp kitchen.yml ./.kitchen/generated_kitchen.yml

rm -rf cookbooks
rm -f Berksfile.lock
berks vendor ./cookbooks

set +o pipefail

# This for loop retries kitchen tests failing because of infrastructure/networking issues
for attempt in $(seq 0 ${KITCHEN_INFRASTRUCTURE_FLAKES_RETRY:-2}); do
  # Test every suite, as we only generate those we want to run
  bundle exec kitchen test ".*" -c -d always 2>&1 | tee /tmp/runlog$attempt
  result=${PIPESTATUS[0]}
  if [ "$result" -eq 0 ]; then
      # if kitchen test succeeded, exit with 0
      exit 0
  else
    if ! invoke kitchen.should-rerun-failed /tmp/runlog$attempt; then
      # if kitchen test failed and shouldn't be rerun, exit with 1
      exit 1
    else
      cp -R ${DD_AGENT_TESTING_DIR}/.kitchen/logs ${DD_AGENT_TESTING_DIR}/.kitchen/logs-${attempt}
    fi
  fi
done

# if we ran out of attempts because of infrastructure/networking issues, exit with 1
echo "Ran out of retry attempts"
exit 1
