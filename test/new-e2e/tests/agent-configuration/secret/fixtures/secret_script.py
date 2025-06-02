#!/usr/bin/env python3
import json
import sys

if __name__ == "__main__":
    agent_payload = sys.stdin.read()
    parsed_agent_payload = json.loads(agent_payload)

    expected_handle = {"hostname": {"value": "e2e.test", "error": None}}

    response = {}
    # Don't simply give back the secrets, verify that Agent sends the correct expected secrets
    for handle_key in expected_handle:
        if handle_key in parsed_agent_payload["secrets"]:
            response[handle_key] = expected_handle[handle_key]

    assert len(parsed_agent_payload["secrets"]) == len(response)
    print(json.dumps(response))
