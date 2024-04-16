# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Run Oracle tests for each database defined in $ORACLE_TEST_CONFIG_DIR/*.cfg

# Usage: run-tests.sh CONFIGURATION_FILES

# Configuration files must define the following variables:
#
# export ORACLE_TEST_USER="<username>"
# export ORACLE_TEST_PASSWORD="<password>"
# export ORACLE_TEST_LEGACY_USER="<legacy
# export ORACLE_TEST_LEGACY_PASSWORD="<legacy_password>"
# export ORACLE_TEST_SERVER="<server>"
# export ORACLE_TEST_PORT="<port>"
# export ORACLE_TEST_SERVICE_NAME="<service_name>"

if [ -z "$1" ]
then
    echo "Configuration files not defined"
    exit 1
fi

files=$1
if [ -d "$files" ]
then
  files="$files/*.cfg"
fi

cd $GOPATH/src/github.com/DataDog/datadog-agent

for f in $files
do
  echo "Starting tests for $f"
  . $f
  go clean -testcache

  # Tests are running sequentially for the predictability of the memory leak test
  gotestsum --jsonfile module_test_output.json --format pkgname --packages=./pkg/collector/corechecks/oracle/... -- -mod=mod -vet=off --tags=oracle_test,oracle,test 
done
