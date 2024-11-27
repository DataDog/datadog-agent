#!/bin/bash
set -e
current_date=$(date +'%Y-%m-%d')
python3 -m pip install --upgrade pip
pip install -r requirements.txt
inv collector.update
inv collector.generate
git config --global user.name "github-actions[bot]"
git config --global user.email "github-actions[bot]@users.noreply.github.com"
git add .
if git diff-index --quiet HEAD; then
  echo "No changes detected"
  changes_detected="false"
else
  changes_detected="true"
fi
if [ "$changes_detected" == "true" ]; then
  git switch -c update-otel-collector-dependencies-"$current_date"
  git commit -m "Update OTel Collector dependencies and generate OTel Agent"
  git push -u origin update-otel-collector-dependencies-"$current_date"

  sudo apt-ge -y update
  sudo apt-get -y install gh

  gh auth login --with-token <<< "$GITHUB_TOKEN"
  gh pr create --title "Update OTel collector dependencies" --body "This PR updates the OTel Collector dependencies to the latest version. Please ensure that all tests pass before marking ready for review." --base main --head update-otel-collector-dependencies-"$current_date" --draft
fi
