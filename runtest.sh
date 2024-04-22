#!/bin/bash

# Infinite loop
while true; do
    # Run the command
    go clean -testcache && go test -mod=mod -gcflags="" -ldflags="-X github.com/DataDog/datadog-agent/pkg/version.Commit=dc4344471c -X github.com/DataDog/datadog-agent/pkg/version.AgentVersion=7.54.0-installer-0.0.1-rc.3+git.32.dc43444 -X github.com/DataDog/datadog-agent/pkg/serializer.AgentPayloadVersion=v5.0.113 -X github.com/DataDog/datadog-agent/pkg/config/setup.ForceDefaultPython=true -X github.com/DataDog/datadog-agent/pkg/config/setup.DefaultPython=3 -r /Users/runner/go/src/github.com/DataDog/datadog-agent/dev/lib '-extldflags=-Wl,-bind_at_load' " -p 3 -race -vet=off  -timeout 180s /Users/arthur.bellal/go/src/github.com/DataDog/datadog-agent/pkg/installer

    # Check the exit status of the command
    if [ $? -ne 0 ]; then
        echo "Command failed"
        break # Exit the loop if the command fails
    fi
done
