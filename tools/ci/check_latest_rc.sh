#!/bin/bash

set -euo pipefail

# 1. list all tags
# 2. clean result and print only the tags
all_tags=$(git ls-remote --tags origin \
	| awk -F/ -v major="7" '$3 ~ ("^"major"\\.[0-9]+\\.[0-9]+-rc\\.[0-9]+$"){print $3}')

# 3. sort and retrieve latest tag
latest_tag=$(echo -e "$all_tags\n$CI_COMMIT_TAG" | sort -V | tail -n1)

echo "latest tag $latest_tag"
echo "current tag $CI_COMMIT_TAG"
if [[ $latest_tag != $CI_COMMIT_TAG ]];
then
	echo "Newer RC tag exists - skipping job."
	exit 1
fi
echo "No newer RC tag found - safe to proceed."
