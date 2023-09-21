#!/usr/bin/env bash

set -euo pipefail

# Get the value of the Git tag "stripe_staging"
PREV_AGENT_SHA=$(git rev-list -n 1 'stripe_staging')

if [ -z "$PREV_AGENT_SHA" ]; then
    echo "Git tag 'stripe_staging' not found"
    exit 1
fi

if [ -z "$CI_COMMIT_SHORT_SHA" ]; then
    echo "Missing new agent tag"
    exit 1
fi

# Initialize changelog variable and unique email addresses list
changelog=""
unique_emails=""

for commit_hash in $(git rev-list "$PREV_AGENT_SHA".."$CI_COMMIT_SHORT_SHA")
do
  # Get the commit message
  commit_message=$(git log --format="%s" -n 1 "$commit_hash")

  # Get the author's email from Git log
  author_email=$(git log -n 1 --pretty=format:"%ae" "$commit_hash")

  # Append commit message and author to changelog
  changelog+="Commit: $commit_message\nAuthor: $author_email\n"

  # Store unique email addresses in the list
  if [ -n "$author_email" ]; then
    echo "$author_email"
    if [[ ! "$unique_emails" =~ $author_email ]]; then
      unique_emails+="\n$author_email"
    fi
  fi

  sleep 2
done

pwd

echo -e "$changelog" > changelog.txt
echo -e "$unique_emails" > unique_emails.txt
