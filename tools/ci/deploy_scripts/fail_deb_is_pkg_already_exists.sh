#!/bin/bash -l

# This script checks all the amd64.deb packages in the working directory against the ones
# in the remote staging apt pool (apt.datad0g.com).
# If a package already exists we exit with error(1) to avoid overwriting a uploaded package.
# # See https://github.com/DataDog/devops/wiki/Datadog-Agent-Release-Overview#apt-repository for why we do this.

# The easiest way to use this script is to provide a glob which searches for all the files to check,
# for instance:
# ./fail_deb_is_pkg_already_exists.sh <glob>
# The glob will be expanded before the script is executed, thus the list of matching files will be
# passed as arguments to the script.

# On bash, if <glob> doesn't match anything, it returns itself when expanded. Thus the script will have the raw <glob> as
# its only argument.
# In that case, the script will exit with an error 1 when checking that the file does not exist (since the
# raw <glob> is not a real file).

set -e

# provide list of files (from glob, for instance) as arguments
if [ "$#" -eq 0 ]; then
    echo "You should provide a list of files as arguments"
    exit 1
fi

# Loop over all files
for FILE in $@; do
  # if the provided file does not exist, exit with error
  [ -e "${FILE}" ] || { echo "No file matching ${FILE} found, aborting verification."; exit 1; }

  if curl --output /dev/null --silent --head --fail "https://s3.amazonaws.com/apt.datad0g.com/pool/d/da/${FILE}"; then
    echo "Error: ${FILE} already exists in the APT pool, you're re-releasing a version."
    exit 1
  fi
done
