"""
Minimal Cloud Run Job workload.
Prints a JSON status line and exits so serverless-init can flush telemetry cleanly.
"""
import json
import os
import sys
import time

result = {
    "status": "ok",
    "service": os.environ.get("DD_SERVICE", "unknown"),
    "env": os.environ.get("DD_ENV", "unknown"),
}
print(json.dumps(result), flush=True)

# Give serverless-init a moment to flush its telemetry queue before the
# process exits.  The ForceCollect() at startup already queued the inventory
# payload; this small sleep lets the forwarder HTTP POST complete.
time.sleep(3)
sys.exit(0)
