# CircleCI

CircleCI is used to run unit tests on Unix env.

## Upgrading Golang version

/!\ Disclaimer: the datadog-agent-runner-circle image should never be used for anything else than CircleCI tests /!\

Change the Golang version in this file `images/runner/Dockerfile`.

Then locally build and push the new image using
`datadog/datadog-agent-runner-circle:go<new golang version>` for the image's
name. You will need write access to that repo on DockerHub (the Agent's team
should have it).

Example:
```bash
cd .circleci/images/runner
docker build --platform=linux/amd64 -t datadog/datadog-agent-runner-circle:go1198 .
docker login
docker push datadog/datadog-agent-runner-circle:go1197
```

Once your image is pushed, update this file:
https://github.com/DataDog/datadog-agent/blob/main/.circleci/config.yml.
Change `image: datadog/datadog-agent-runner-circle:goXXXX` for the tag you
just pushed.

Push your change as a new PR to see if CircleCI is still green.

If everything is green, get a review and merge the PR.
