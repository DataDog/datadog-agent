#!/usr/bin/env python3
"""Load driver for synthetic Network Path perf testing.

Creates / lists / deletes synthetic Network Path tests targeting the
`experiment:perf-2026-05` tag. Both VMs in the perf experiment have that tag,
so every test in the pool runs on every agent simultaneously — exactly
identical load per VM, no sharding.

Usage:
    python load_driver.py create <N>      # create N tests
    python load_driver.py list            # list current perf-test-* tests
    python load_driver.py teardown        # delete all perf-test-* tests

Env vars:
    DD_API_KEY  - Datadog API key                       (required)
    DD_APP_KEY  - Datadog application key               (required, write scope)
    DD_SITE     - Datadog site, e.g. datadoghq.com,     (default: datadoghq.com)
                  us3.datadoghq.com, datadoghq.eu

Workflow:
    python load_driver.py create 1        # smoke test: confirm a single test
                                          # appears in Datadog UI + runs on
                                          # both agents
    python load_driver.py teardown        # clean up smoke test
    python load_driver.py create 10       # start the ramp at N=10
    # ...wait dwell, record observations in notebook...
    python load_driver.py teardown
    python load_driver.py create 50
    # ...and so on.

CAVEATS:
- The synthetic Network Path API payload schema (subtype name, config
  structure, location-by-tag semantics) may have shifted between Agent
  releases. Run with N=1 first; if create 4xx's, the response body tells
  you what the server actually expects — adjust `create_test()` accordingly.
- Teardown deletes ANYTHING whose name starts with `perf-test-`. If you
  have unrelated synthetic tests with that prefix, change `PREFIX` below.
"""
import os
import random
import sys

import requests

PREFIX = "perf-test"
EXPERIMENT_TAG = "experiment:perf-2026-05"

# Mix of well-connected destinations on the public internet. Each created
# test is randomly assigned one so path-trace results vary across tests
# (rather than every agent hammering the same single endpoint).
TARGETS = [
    ("www.google.com", 443),
    ("api.datadoghq.com", 443),
    ("github.com", 443),
    ("www.cloudflare.com", 443),
    ("www.microsoft.com", 443),
    ("www.amazon.com", 443),
    ("1.1.1.1", 443),
    ("8.8.8.8", 443),
]


def site() -> str:
    return os.environ.get("DD_SITE", "datadoghq.com")


def auth_headers() -> dict:
    try:
        return {
            "DD-API-KEY": os.environ["DD_API_KEY"],
            "DD-APPLICATION-KEY": os.environ["DD_APP_KEY"],
            "Content-Type": "application/json",
        }
    except KeyError as e:
        sys.exit(f"missing env var: {e}")


def api_url(path: str) -> str:
    return f"https://api.{site()}{path}"


def create_test(name: str, host: str, port: int) -> dict:
    """Create one synthetic Network Path test.

    The `locations` field uses a tag selector so every agent carrying that
    tag picks up the test. Verify the exact field name (locations vs.
    runner_tags vs. private_locations) against the Synthetics API docs if
    this 4xx's.
    """
    body = {
        "name": name,
        "type": "api",
        "subtype": "network_path",  # <-- verify against docs
        "config": {
            "request": {
                "host": host,
                "port": port,
                "protocol": "tcp",
            },
            "assertions": [],
        },
        "options": {
            "tick_every": 60,
            "min_failure_duration": 0,
            "min_location_failed": 1,
            "monitor_priority": 5,
        },
        "locations": [EXPERIMENT_TAG],
        "tags": ["campaign:netpath-perf", "owner:ken"],
        "message": "perf-test (auto-created)",
        "status": "live",
    }
    r = requests.post(
        api_url("/api/v1/synthetics/tests/api"),
        json=body,
        headers=auth_headers(),
        timeout=30,
    )
    if r.status_code >= 300:
        sys.exit(f"create {name}: HTTP {r.status_code}\n{r.text}")
    return r.json()


def list_perf_tests() -> list:
    r = requests.get(
        api_url("/api/v1/synthetics/tests"),
        headers=auth_headers(),
        timeout=30,
    )
    r.raise_for_status()
    tests = r.json().get("tests", [])
    return [t for t in tests if t.get("name", "").startswith(PREFIX)]


def cmd_create(n: int) -> None:
    print(f"create: {n} tests, target tag {EXPERIMENT_TAG}")
    for i in range(n):
        host, port = random.choice(TARGETS)
        name = f"{PREFIX}-{i:04d}-{host.replace('.', '-')}-{port}"
        create_test(name, host, port)
        if (i + 1) % 25 == 0:
            print(f"  created {i + 1}/{n}")
    print(f"create: done ({n} tests)")


def cmd_list() -> None:
    tests = list_perf_tests()
    print(f"{len(tests)} {PREFIX}-* tests:")
    for t in tests:
        print(f"  {t['public_id']}  {t['name']}")


def cmd_teardown() -> None:
    tests = list_perf_tests()
    if not tests:
        print("teardown: no tests to delete")
        return
    public_ids = [t["public_id"] for t in tests]
    print(f"teardown: deleting {len(public_ids)} tests")
    r = requests.post(
        api_url("/api/v1/synthetics/tests/delete"),
        json={"public_ids": public_ids},
        headers=auth_headers(),
        timeout=60,
    )
    if r.status_code >= 300:
        sys.exit(f"teardown: HTTP {r.status_code}\n{r.text}")
    print(f"teardown: deleted {len(public_ids)}")


def main() -> None:
    args = sys.argv[1:]
    if len(args) == 2 and args[0] == "create":
        cmd_create(int(args[1]))
    elif len(args) == 1 and args[0] == "list":
        cmd_list()
    elif len(args) == 1 and args[0] == "teardown":
        cmd_teardown()
    else:
        print(__doc__, file=sys.stderr)
        sys.exit(2)


if __name__ == "__main__":
    main()
