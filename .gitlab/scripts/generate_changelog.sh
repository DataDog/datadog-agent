#!/usr/bin/env bash

set -euo pipefail

# Get the value of the Git tag "stripe_staging"
old_agent_tag=$(git tag -l 'stripe_staging' | tail -n 1)

if [ -z "$old_agent_tag" ]; then
    echo "Git tag 'stripe_staging' not found"
    exit 1
fi

if [ -z "$2" ]; then
    echo "Missing new agent tag"
    exit 1
fi

# Declare an associative array to map email addresses to Slack handles
declare -A email_to_slack

# Initialize changelog variable
changelog=""

for commit_hash in $(git rev-list "$1".."$2")
do
  # Get the author's email from Git log
  author_email=$(git log -n 1 --pretty=format:"%ae" "$commit_hash")

  # Fetch PR information using 'gh'
  pr_info=$(gh search prs "$commit_hash" --repo 'DataDog/datadog-agent' --label 'component/system-probe' --merged --json 'title,url,author,number' --template "{{range .}}{{printf \"%v %v %v %v\\n\" .title \"$author_email\" .author.login .url}}{{end}}")

  # Append PR info to changelog
  changelog+="$pr_info"

  # Convert email to Slack handle and store in the associative array
  if [ -n "$author_email" ]; then
    slack_handle=$(echo "$author_email" | email2slackid || echo "")
    if [ -n "$slack_handle" ]; then
      email_to_slack["$author_email"]=$slack_handle
    fi
  fi

  sleep 2
done

# Generate the list of unique Slack handles
unique_slack_handles=""
for email in "${!email_to_slack[@]}"
do
  SLACK_AUTHOR="${email_to_slack["$email"]}"
  unique_slack_handles+="@$SLACK_AUTHOR "
done

# Use the 'changelog' variable as needed, e.g., post it to Slack or append it to your message.
# Append the unique list of Slack handles to the end of the changelog
postmessage "system-probe-ops" "Changelog:\n$changelog\nUnique Slack Handles: $unique_slack_handles"
