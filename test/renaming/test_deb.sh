#!/bin/bash

#
# Script to test whether the StackState agent contains no visible references to DataDog
# (only to StackState)
#
# Arguments:
#   1) package name
#

set -e

# parameter check
if [ "$#" -ne 1 ];  then
  echo "Parameter error."
  echo "Usage: ./test/sh <package>"
  echo "Example: ./test.sh pkg/stackstate-agent_1.0.0.git.3.d95a036-1_amd64.deb"
  exit 1
fi

# set variables based on parameters
ARTIFACT=$1
PROJECT_DIR=`pwd`

# artifact present?
echo "Checking artifact's existence."
if [ ! -e ${ARTIFACT} ]; then
  echo "Artifact not found at ${ARTIFACT}."
  exit 2
fi

cp ${ARTIFACT} agent.deb

rm -rf unpacked
mkdir unpacked
cd unpacked

ar xv ../agent.deb
tar xvf control.tar.gz || true
tar xvf data.tar.gz || true

# Stuff in embedded is not important
LICENSE_DIR1="/opt/stackstate-agent/licenses/"
LICENSE_DIR2="/opt/stackstate-agent/LICENSES/"

find . -iname \*datadog\* \
  | grep -v "$LICENSE_DIR1" \
  | grep -v "$LICENSE_DIR2" \
  | grep -v "/opt/stackstate-agent/bin/agent/dist/views/private/images/datadog_icon_white.svg" \
  | grep -v "/opt/stackstate-agent/LICENSES/go_dep-gopkg.in_zorkian_go-datadog-api" \
  | grep -v "/opt/stackstate-agent/LICENSES/go_dep-github.com_DataDog_agent-payload" \
  | grep -v "/opt/stackstate-agent/LICENSES/go_dep-github.com_DataDog_gohai" \
  | grep -v "/opt/stackstate-agent/LICENSES/go_dep-github.com_DataDog_zstd" \
  | grep -v "/opt/stackstate-agent/embedded/lib/python2.7/site-packages/stackstate_checks/stubs/datadog_agent.py" \
  | grep -v "/opt/stackstate-agent/embedded/lib/python2.7/site-packages/stackstate_checks/base/stubs/datadog_agent.py" \
  | grep -v "/opt/stackstate-agent/embedded/lib/python2.7/site-packages/datadog_a7" \
  | tee -a out.txt
find . -iname \*dd-\* | tee -a out.txt

grep -R "datadog_checks" ./opt/stackstate-agent/embedded/ \
  | grep -v "datadog_checks_shared" \
  | tee -a out.txt \

echo "Output:"
cat out.txt
echo "end"

# Verify we found no references to datadog we did not expect
if [ -s out.txt ]
then
  echo "Please fix branding: there is still something using (dd- | datadog) prefix"
  exit 1
else
  echo "Branding was successful"
fi

exit 0
