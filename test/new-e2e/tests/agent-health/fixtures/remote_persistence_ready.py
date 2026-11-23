# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

"""Wait for the remote persistence backend through its HTTPS Service."""

import ssl
import sys
import time
import urllib.error
import urllib.request

url, ca_file = sys.argv[1:]
context = ssl.create_default_context(cafile=ca_file)
deadline = time.monotonic() + 60
last_error = "no successful response"

while True:
    remaining = deadline - time.monotonic()
    if remaining <= 0:
        raise SystemExit(f"remote persistence backend not ready at {url}: {last_error}")
    try:
        with urllib.request.urlopen(url, context=context, timeout=min(5, remaining)) as response:
            if response.status == 200:
                break
            last_error = f"HTTP status {response.status}"
    except (urllib.error.URLError, TimeoutError) as error:
        last_error = str(error)
    time.sleep(min(1, max(0, deadline - time.monotonic())))
