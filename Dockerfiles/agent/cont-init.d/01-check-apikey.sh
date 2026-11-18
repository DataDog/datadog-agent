#!/bin/bash

# Do not allow starting without authentication configured.
# Either DD_API_KEY or DD_DELEGATED_AUTH_ORG_UUID (Workload Identity Federation) must be set.
if [[ -z "${DD_API_KEY}" ]] && [[ -z "${DD_DELEGATED_AUTH_ORG_UUID}" ]]; then
    echo ""
    echo "=================================================================================="
    echo "You must set either a DD_API_KEY or DD_DELEGATED_AUTH_ORG_UUID environment"
    echo "variable to run the Datadog Agent container."
    echo "=================================================================================="
    echo ""
    exit 1
fi

# Do not allow starting with the all-zeros placeholder API key (set by dd-auth 2.x unless a
# real key is requested) when payloads go to Datadog: the Agent would run but every payload
# would be rejected with a 403. A custom intake, such as fakeintake in tests, still accepts it.
if [[ "${DD_API_KEY}" == "00000000000000000000000000000000" ]] \
    && [[ -z "${DD_DELEGATED_AUTH_ORG_UUID}${DD_DD_URL}${DD_URL}${DD_ADDITIONAL_ENDPOINTS}" ]]; then
    echo ""
    echo "=================================================================================="
    echo "DD_API_KEY is the placeholder 00000000000000000000000000000000, which Datadog"
    echo "rejects. Set DD_API_KEY to a valid API key to run the Datadog Agent container."
    echo "With dd-auth 2.x, set DD_AUTH_API_KEY=1 (or pass --api-key) to get one."
    echo "=================================================================================="
    echo ""
    exit 1
fi
