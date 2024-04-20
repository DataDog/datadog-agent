#!/bin/bash

# This script sets up the environment and then runs the test kitchen itself.

# http://redsymbol.net/articles/unofficial-bash-strict-mode/
IFS=$'\n\t'
set -euo pipefail

# Ensure that the ssh key is never reused between tests
if [ -f "$(pwd)/ssh-key" ]; then
  rm ssh-key
fi
if [ -f "$(pwd)/ssh-key.pub" ]; then
  rm ssh-key.pub
fi
# Set a PARENT_DIR variable to call the aws_ssm wrapper in both local and CI contexts
pushd ../..
if [ -n "$CI_PROJECT_DIR" ]; then
  PARENT_DIR="$CI_PROJECT_DIR"
else
  PARENT_DIR="$(pwd)"
fi
popd

# in docker we cannot interact to do this so we must disable it
mkdir -p ~/.ssh
[[ -f /.dockerenv ]] && echo -e "Host *\n\tStrictHostKeyChecking no\n\n" > ~/.ssh/config

if [ "$KITCHEN_PROVIDER" == "azure" ]; then
  # Generating SSH keys to connect to Azure VMs

  ssh-keygen -f "$(pwd)/ed25519-key" -P "" -a 100 -t ed25519
  KITCHEN_ED25519_SSH_KEY_PATH="$(pwd)/ed25519-key"
  export KITCHEN_ED25519_SSH_KEY_PATH

  # show that the ed25519 ssh key is there
  ls "$(pwd)/ed25519-key"

  ssh-keygen -f "$(pwd)/rsa-key" -P "" -t rsa -b 2048
  KITCHEN_RSA_SSH_KEY_PATH="$(pwd)/rsa-key"
  export KITCHEN_RSA_SSH_KEY_PATH

  # show that the rsa ssh key is there
  ls "$(pwd)/rsa-key"

  # start the ssh-agent and add the keys
  eval "$(ssh-agent -s)"
  ssh-add "$KITCHEN_RSA_SSH_KEY_PATH"
  ssh-add "$KITCHEN_ED25519_SSH_KEY_PATH"

  # Setup the azure credentials, grabbing them from AWS if they do not exist in the environment already
  # If running locally, they should be imported into the environment

  # These should not be printed out
  set +x
  if [ -z ${AZURE_CLIENT_ID+x} ]; then
    AZURE_CLIENT_ID=$($PARENT_DIR/tools/ci/aws_ssm_get_wrapper.sh $KITCHEN_AZURE_CLIENT_ID_SSM_NAME)
    # make sure whitespace is removed
    AZURE_CLIENT_ID="$(echo -e "${AZURE_CLIENT_ID}" | tr -d '[:space:]')"
    export AZURE_CLIENT_ID
  fi
  if [ -z ${AZURE_CLIENT_SECRET+x} ]; then
    AZURE_CLIENT_SECRET=$($PARENT_DIR/tools/ci/aws_ssm_get_wrapper.sh $KITCHEN_AZURE_CLIENT_SECRET_SSM_NAME)
    # make sure whitespace is removed
    AZURE_CLIENT_SECRET="$(echo -e "${AZURE_CLIENT_SECRET}" | tr -d '[:space:]')"
    export AZURE_CLIENT_SECRET
  fi
  if [ -z ${AZURE_TENANT_ID+x} ]; then
    AZURE_TENANT_ID=$($PARENT_DIR/tools/ci/aws_ssm_get_wrapper.sh $KITCHEN_AZURE_TENANT_ID_SSM_NAME)
    # make sure whitespace is removed
    AZURE_TENANT_ID="$(echo -e "${AZURE_TENANT_ID}" | tr -d '[:space:]')"
    export AZURE_TENANT_ID
  fi
  if [ -z ${AZURE_SUBSCRIPTION_ID+x} ]; then
    AZURE_SUBSCRIPTION_ID=$($PARENT_DIR/tools/ci/aws_ssm_get_wrapper.sh $KITCHEN_AZURE_SUBSCRIPTION_ID_SSM_NAME)
    # make sure whitespace is removed
    AZURE_SUBSCRIPTION_ID="$(echo -e "${AZURE_SUBSCRIPTION_ID}" | tr -d '[:space:]')"
    export AZURE_SUBSCRIPTION_ID
  fi

  if [ -z ${AZURE_SUBSCRIPTION_ID+x} ] || [ -z ${AZURE_TENANT_ID+x} ] || [ -z ${AZURE_CLIENT_SECRET+x} ] || [ -z ${AZURE_CLIENT_ID+x} ]; then
    printf "You are missing some of the necessary credentials. Exiting."
    exit 1
  fi

  # Create the Azure credentials file as requried by the kitchen-azurerm driver
  mkdir -p ~/.azure/
  (echo "<% subscription_id=\"$AZURE_SUBSCRIPTION_ID\"; client_id=\"$AZURE_CLIENT_ID\"; client_secret=\"$AZURE_CLIENT_SECRET\"; tenant_id=\"$AZURE_TENANT_ID\"; %>" && cat azure-creds.erb) | erb > ~/.azure/credentials

elif [ "$KITCHEN_PROVIDER" == "ec2" ]; then
  echo "using ec2 kitchen provider"

  # Setup the AWS credentials: grab the ED25519 ssh key that is needed to connect to Amazon Linux 2022 instances
  # See: https://github.com/test-kitchen/kitchen-ec2/issues/588
  # Note: this issue happens even when allowing RSA keys in the ssh service of the remote host (which was the fix we did for Ubuntu 22.04),
  # therefore using the auto-generated SSH key is not possible at all.

  # These should not be printed out
  set +x
  if [ -z ${KITCHEN_EC2_SSH_KEY_ID+x} ]; then
    export KITCHEN_EC2_SSH_KEY_ID="datadog-agent-kitchen"
    export KITCHEN_EC2_SSH_KEY_PATH="$(pwd)/aws-ssh-key"
    touch $KITCHEN_EC2_SSH_KEY_PATH && chmod 600 $KITCHEN_EC2_SSH_KEY_PATH
    $PARENT_DIR/tools/ci/aws_ssm_get_wrapper.sh $KITCHEN_EC2_SSH_KEY_SSM_NAME > $KITCHEN_EC2_SSH_KEY_PATH
  fi
fi

# Generate a password to use for the windows servers
if [ -z ${SERVER_PASSWORD+x} ]; then
  export SERVER_PASSWORD="$(tr -dc A-Za-z0-9 < /dev/urandom | head -c32)0aZ"
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

KITCHEN_IMAGE_SIZE="${KITCHEN_IMAGE_SIZE:-}"

invoke -e kitchen.genconfig --platform="$KITCHEN_PLATFORM" --osversions="$KITCHEN_OSVERS" --provider="$KITCHEN_PROVIDER" --arch="${KITCHEN_ARCH:-x86_64}" --imagesize="${KITCHEN_IMAGE_SIZE}" --testfiles="$1" ${KITCHEN_FIPS:+--fips} --platformfile=platforms.json

bundle exec kitchen diagnose --no-instances --loader

## copy the generated kitchen.yml to the .kitchen directory so it'll be included
## in the artifacts (for debugging when necessary)
cp kitchen.yml ./.kitchen/generated_kitchen.yml

rm -rf cookbooks
rm -f Berksfile.lock
berks vendor ./cookbooks

set +o pipefail

# Initially test every suite, as we only generate those we want to run
test_suites=".*"
# This for loop retries kitchen tests failing because of infrastructure/networking issues
for attempt in $(seq 0 "${KITCHEN_INFRASTRUCTURE_FLAKES_RETRY:-2}"); do
  bundle exec kitchen verify "$test_suites" -c -d always 2>&1 | tee "/tmp/runlog${attempt}"
  result=${PIPESTATUS[0]}
  # Before destroying the kitchen machines, get the list of failed suites,
  # as their status will be reset to non-failing once they're destroyed.
  # failing_test_suites is a newline-separated list of the failing test suite names.
  failing_test_suites=$(bundle exec kitchen list --no-log-overwrite --json | jq -cr "[ .[] | select( .last_error != null ) ] | map( .instance ) | .[]")

  # Then, destroy the kitchen machines
  # Do not fail on kitchen destroy, it breaks the infra failures filter
  set +e
  bundle exec kitchen destroy "$test_suites" --no-log-overwrite
  destroy_result=$?
  set -e

  # If the destory operation fails, it is not safe to continue running kitchen
  # so we just exit with an infrastructure failure message.
  if [ "$destroy_result" -ne 0 ]; then
    echo "Failure while destroying kitchen infrastructure, skipping retries"
    break
  fi

  if [ "$result" -eq 0 ]; then
      echo "Kitchen test succeeded exiting 0"
      exit 0
  else
    if ! invoke kitchen.should-rerun-failed "/tmp/runlog${attempt}" ; then
      # if kitchen test failed and shouldn't be rerun, exit with 1
      echo "Kitchen tests failed and it should not be an infrastructure problem"
      exit 1
    else
      cp -R "${DD_AGENT_TESTING_DIR}"/.kitchen/logs "${DD_AGENT_TESTING_DIR}/.kitchen/logs-${attempt}"
      # Only keep test suites that have a non-null error code
      # Build the result as a regexp: "test_suite1|test_suite2|test_suite3", as kitchen only
      # supports one instance name or a regexp as argument.
      test_suites=$(echo -n "$failing_test_suites" | tr '\n' '|')
    fi
  fi
done


# if we ran out of attempts because of infrastructure/networking issues, exit with 1
echo "Ran out of retry attempts"
echo "ERROR: The kitchen tests failed due to infrastructure failures."
exit 1
