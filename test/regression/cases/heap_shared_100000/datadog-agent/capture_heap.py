#!/usr/bin/env python3
"""Capture timestamped Agent heap evidence in SMP target logs without forcing GC."""

import base64
import hashlib
import json
import os
import time
import urllib.request
import uuid

BASE = os.environ.get("HEAP_CAPTURE_URL", "http://127.0.0.1:5000")
START = time.monotonic()
SESSION = uuid.uuid4().hex
FIELDS = (
    "HeapAlloc",
    "HeapSys",
    "HeapIdle",
    "HeapInuse",
    "HeapReleased",
    "HeapObjects",
    "TotalAlloc",
    "NextGC",
    "LastGC",
    "NumGC",
    "PauseTotalNs",
    "Sys",
)


def emit(kind, **fields):
    print(
        json.dumps(
            dict(
                diagnostic="endpoint_routing_heap",
                kind=kind,
                session=SESSION,
                case=os.environ.get("HEAP_CASE"),
                variant=os.environ.get("HEAP_VARIANT"),
                replicate=os.environ.get("HEAP_REPLICATE"),
                job=os.environ.get("HEAP_JOB"),
                timestamp=time.time(),
                elapsed=time.monotonic() - START,
                **fields,
            )
        ),
        flush=True,
    )


def get(path):
    with urllib.request.urlopen(BASE + path, timeout=15) as response:
        return response.read()


def memory():
    stats = json.loads(get("/debug/vars"))["memstats"]
    return {key: stats[key] for key in FIELDS}


def resident(pid):
    try:
        with open(f"/proc/{pid}/smaps_rollup") as source:
            return {
                key: int(value.split()[0]) * 1024
                for key, value in (line.split(":", 1) for line in source if ":" in line)
                if key in ("Rss", "Pss", "Private_Clean", "Private_Dirty")
            }
    except OSError:
        return None


def main():
    first = float(os.environ.get("HEAP_CAPTURE_FIRST_SECONDS", "75"))
    interval = float(os.environ.get("HEAP_CAPTURE_INTERVAL_SECONDS", "37"))
    duration = float(os.environ.get("HEAP_CAPTURE_DURATION_SECONDS", "3600"))
    next_heap = first
    sequence = 0
    while time.monotonic() - START < duration:
        try:
            emit(
                "memory",
                memstats=memory(),
                agent_resident=resident(os.environ.get("HEAP_AGENT_PID", str(os.getppid()))),
                collector_resident=resident(os.getpid()),
            )
            telemetry = get("/telemetry").decode()
            queues = [
                line
                for line in telemetry.splitlines()
                if not line.startswith("#")
                and (
                    "transactions_retry_queue_size" in line
                    or "transactions_high_priority_queue_full" in line
                    or "current_mem_size_in_bytes" in line
                    or "transactions_retries" in line
                )
            ]
            emit("queues", metrics=queues)
            if time.monotonic() - START >= next_heap:
                before = memory()
                # No gc=1: observe the normal workload rather than changing its GC cadence.
                profile = get("/debug/pprof/heap")
                after = memory()
                encoded = base64.b64encode(profile).decode("ascii")
                chunks = [encoded[i : i + 12000] for i in range(0, len(encoded), 12000)]
                digest = hashlib.sha256(profile).hexdigest()
                emit(
                    "heap_start",
                    sequence=sequence,
                    sha256=digest,
                    chunks=len(chunks),
                    bytes=len(profile),
                    before=before,
                    after=after,
                )
                for index, chunk in enumerate(chunks):
                    emit("heap_chunk", sequence=sequence, index=index, data=chunk)
                emit("heap_end", sequence=sequence, sha256=digest)
                sequence += 1
                next_heap += interval
        except Exception as error:
            emit("error", error=str(error))
        time.sleep(5)


if __name__ == "__main__":
    main()
