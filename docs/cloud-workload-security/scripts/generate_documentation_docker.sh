#!/usr/bin/env bash

TAG_NAME="ddagent_security_agent_docs_builder"

docker build . --file ./docs/cloud-workload-security/scripts/Dockerfile --tag "$TAG_NAME"
docker run --rm -v $(pwd):/go/src/github.com/DataDog/datadog-agent "$TAG_NAME" inv -e security-agent.generate-cws-documentation --go-generate
