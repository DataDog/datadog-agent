#!/usr/bin/env -S uv run --script --quiet
# /// script
# requires-python = ">=3.10,<3.14"
# dependencies = []
# ///
"""OpenMetrics differential-testing Python sidecar.

Long-lived stdin/stdout process. Two request modes:

1) ONE-SHOT (original): single scrape, fresh check instance per request.

    {"endpoint": "http://127.0.0.1:NNNN/metrics", "instance": {...}}
    -> {"submissions": [...], "error": null | "..."}

2) SESSION (added for stateful two-scrape testing): open a session, run
   multiple scrapes against the same check instance (so flush_first_value
   and other scrape-to-scrape state are preserved), then close.

    {"op": "open_session", "session_id": "<id>", "endpoint": "...", "instance": {...}}
    -> {"session_id": "...", "ready": true}

    {"op": "scrape", "session_id": "<id>"}
    -> {"submissions": [...], "error": null | "..."}

    {"op": "close_session", "session_id": "<id>"}
    -> {"closed": true}

The two modes coexist; messages without an "op" field are dispatched as
one-shot, preserving backward compatibility with the original protocol.

The caller (the Go test) owns the HTTP server, so both Go and Python
scrapers see byte-identical payloads. Throwaway debugging tool — not wired
into CI.
"""

import json
import sys
import time
import traceback

from datadog_checks.base.checks.openmetrics.v2.base import OpenMetricsBaseCheckV2
from datadog_checks.base.stubs import datadog_agent

# The production agent records its own start time and exposes it via
# `datadog_agent.get_process_start_time()`. The sidecar's stubbed agent
# defaults to 0 (Unix epoch), which makes `use_process_start_time` behave
# very differently from production: any non-zero process_start_time_seconds
# value in a payload satisfies `process_start > agent_start`, so the first-
# scrape handler always flushes.
#
# Seed the stub with the sidecar's actual startup time so the comparison
# matches what the real agent would compute. Tests targeting
# use_process_start_time should use payload values that are either clearly
# old (< sidecar startup) or clearly future (> sidecar startup); the
# semantics then match Go's `pkgconfigsetup.StartTime.Unix()` check.
datadog_agent.set_process_start_time(time.time())

# Capture submissions by patching the BASE class BEFORE any check instance
# is created — OpenMetrics transformers grab bound methods at config time,
# so patching after __init__ is too late.
#
# We replace `gauge`/`count`/.../`service_check` directly, which means we *skip*
# AgentCheck's normal `_format_namespace` step. To stay byte-equivalent with the
# Go side (which always emits namespaced names), we re-apply the namespace here,
# honouring the `raw=True` opt-out that the OpenMetrics transformers use for
# metrics that shouldn't be prefixed (e.g. user-supplied `raw_metric_prefix`).
_captured: list[dict] = []


def _format_name(self, name: str, raw: bool) -> str:
    ns = getattr(self, "__NAMESPACE__", "") or ""
    if raw or not ns:
        return name
    return f"{ns}.{name}"


def _mk(kind: str):
    def fn(self, name, value=None, tags=None, hostname=None, raw=False, **kw):
        _captured.append(
            {
                "kind": kind,
                "name": _format_name(self, name, raw),
                "value": value,
                "tags": sorted(tags or []),
                "hostname": hostname or "",
            }
        )

    return fn


for _kind in ("gauge", "count", "rate", "histogram", "historate"):
    setattr(OpenMetricsBaseCheckV2, _kind, _mk(_kind))


# monotonic_count gets a dedicated handler that records the
# `flush_first_value` kwarg — stateful tests use it to verify the flag
# toggles correctly across scrapes.
def _monotonic_count(self, name, value=None, tags=None, hostname=None, raw=False, flush_first_value=False, **kw):
    entry = {
        "kind": "monotonic_count",
        "name": _format_name(self, name, raw),
        "value": value,
        "tags": sorted(tags or []),
        "hostname": hostname or "",
    }
    if flush_first_value:
        entry["flush_first_value"] = True
    _captured.append(entry)


OpenMetricsBaseCheckV2.monotonic_count = _monotonic_count


def _service_check(self, name, status, tags=None, hostname=None, message=None, raw=False, **kw):
    _captured.append(
        {
            "kind": "service_check",
            "name": _format_name(self, name, raw),
            "value": status,
            "tags": sorted(tags or []),
            "hostname": hostname or "",
            "message": message or "",
        }
    )


OpenMetricsBaseCheckV2.service_check = _service_check


class _Probe(OpenMetricsBaseCheckV2):
    # Fixed namespace so metric names are stable across runs. Real callers can
    # override via the instance config's `namespace` (it wins over __NAMESPACE__).
    __NAMESPACE__ = "diff"


# Session storage. Keyed by caller-provided session_id. Single-threaded
# access — callers serialize requests through stdin.
_sessions: dict[str, _Probe] = {}


def _run_one(req: dict) -> dict:
    """One-shot scrape: fresh check instance every call. Legacy behaviour."""
    _captured.clear()
    endpoint = req["endpoint"]
    instance = dict(req.get("instance") or {})
    instance["openmetrics_endpoint"] = endpoint
    try:
        check = _Probe("diff", {}, [instance])
        # `.run()` returns "" on success or a JSON error blob on failure.
        err = check.run()
    except Exception:  # noqa: BLE001 — we surface anything to the Go side as a string
        err = traceback.format_exc()
    return {"submissions": list(_captured), "error": err or None}


def _open_session(req: dict) -> dict:
    """Create a long-lived check instance pinned to the given endpoint+instance.

    The check's scraper state (most importantly `flush_first_value`) is
    preserved across subsequent `scrape` calls until `close_session`.
    """
    sid = req.get("session_id")
    if not sid:
        return {"error": "open_session: session_id required"}
    if sid in _sessions:
        return {"error": f"open_session: session_id {sid!r} already exists"}
    endpoint = req.get("endpoint")
    if not endpoint:
        return {"error": "open_session: endpoint required"}
    instance = dict(req.get("instance") or {})
    instance["openmetrics_endpoint"] = endpoint
    try:
        check = _Probe("diff", {}, [instance])
    except Exception:  # noqa: BLE001
        return {"error": traceback.format_exc()}
    _sessions[sid] = check
    return {"session_id": sid, "ready": True}


def _scrape_session(req: dict) -> dict:
    """Run one scrape using a previously-opened session."""
    sid = req.get("session_id")
    if not sid:
        return {"submissions": [], "error": "scrape: session_id required"}
    check = _sessions.get(sid)
    if check is None:
        return {"submissions": [], "error": f"scrape: unknown session {sid!r}"}
    _captured.clear()
    try:
        err = check.run()
    except Exception:  # noqa: BLE001
        err = traceback.format_exc()
    return {"submissions": list(_captured), "error": err or None}


def _close_session(req: dict) -> dict:
    """Drop a session. Idempotent."""
    sid = req.get("session_id")
    _sessions.pop(sid, None)
    return {"closed": True}


def _dispatch(req: dict) -> dict:
    op = req.get("op")
    if op is None:
        return _run_one(req)
    if op == "open_session":
        return _open_session(req)
    if op == "scrape":
        return _scrape_session(req)
    if op == "close_session":
        return _close_session(req)
    return {"error": f"unknown op: {op!r}"}


def main() -> None:
    # Line-buffered stdout so the Go side can read incrementally.
    sys.stdout.reconfigure(line_buffering=True)
    # Announce readiness so the Go side can wait for it instead of racing.
    print(json.dumps({"ready": True}), flush=True)
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except json.JSONDecodeError as e:
            print(json.dumps({"submissions": [], "error": f"bad request json: {e}"}), flush=True)
            continue
        resp = _dispatch(req)
        print(json.dumps(resp, default=str), flush=True)


if __name__ == "__main__":
    main()
