#!/bin/sh

# To use orchestrion with go tests, we need Datadog environment variables which conflicts with the ones used by the tests.
# Process:
# - dda inv test
# - gotestsum toolexec
#   - orchestrion
#     - sanitize
#       - go test


# ORCHESTRION_LOG_LEVEL=debug GOWORK=off orchestrion go test .
# ORCHESTRION_LOG_LEVEL=trace GOWORK=off GOFLAGS="$GOFLAGS '-toolexec=orchestrion toolexec'" go test
# ORCHESTRION_LOG_LEVEL=trace GOWORK=off go test . -toolexec='orchestrion toolexec'
# ORCHESTRION_LOG_LEVEL=info GOWORK=off gotestsum --raw-command -- orchestrion go test -json -shuffle=on .
# ORCHESTRION_LOG_LEVEL=info GOWORK=off gotestsum -- -toolexec='orchestrion toolexec' . # ?
# ORCHESTRION_LOG_LEVEL=info GOWORK=off GOFLAGS="$GOFLAGS '-toolexec=orchestrion toolexec'" gotestsum -- .

# Each ORCHESTRION_DD_* variables are forwarded to orchestrion to avoid conflicts in the tests
# We need these variables like the api key etc.
for var in $(env | grep ^ORCHESTRION_DD_); do
    export "${var#ORCHESTRION_DD_}=${!var}"
done
orchestrion toolexec "$@"
