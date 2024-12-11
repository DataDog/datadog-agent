#!/bin/bash -e

filename="${HOME}/comment.md"
echo "filename=$filename" >> "$GITHUB_OUTPUT"

if [ "${VAR_COLD_START}" != 0 ]; then
  echo -n ":warning::rotating_light: Warning, " > "$filename"
else
  echo -n ":inbox_tray: :loudspeaker: Info, " > "$filename"
fi

cat >> "$filename" << EOL
this pull request increases the binary size of serverless extension by ${VAR_DIFF} bytes. Each MB of binary size increase means about 10ms of additional cold start time, so this pull request would increase cold start time by ${VAR_COLD_START}ms.

<details>
<summary>Debug info</summary>

If you have questions, we are happy to help, come visit us in the [#serverless](https://dd.slack.com/archives/CBWDFKWV8) slack channel and provide a link to this comment.

EOL

if [ -n "$VAR_DEPS" ]; then
    cat >> "$filename" << EOL

These dependencies were added to the serverless extension by this pull request:

\`\`\`
${VAR_DEPS}
\`\`\`

View dependency graphs for each added dependency in the [artifacts section](https://github.com/DataDog/datadog-agent/actions/runs/${VAR_RUN_ID}#artifacts) of the github action.

EOL
fi

cat >> "$filename" << EOL

We suggest you consider adding the \`!serverless\` build tag to remove any new dependencies not needed in the serverless extension.

</details>

EOL

echo "Will post comment with message:"
echo
cat "$filename"
