#!/usr/bin/env -S uv run --script --quiet
# /// script
# requires-python = ">=3.10,<3.14"
# dependencies = [
#   "datadog-checks-base[deps,json] @ file:///home/bits/dd/integrations-core/datadog_checks_base",
# ]
# ///
"""OpenMetrics differential-testing Python sidecar.

Long-lived stdin/stdout process. Each line on stdin is a JSON request of the form:

    {"endpoint": "http://127.0.0.1:NNNN/metrics", "instance": {...}}

For each request the sidecar instantiates a fresh `OpenMetricsBaseCheckV2`,
runs it once via `.run()` (which fires `check_initializations` — calling
`.check()` directly leaves `self.scrapers` empty and submits nothing), and
emits a single JSON line on stdout of the form:

    {"submissions": [ {kind, name, value, tags, hostname, ...}, ... ],
     "error": null | "..."}

The caller (the Go test) owns the HTTP server, so both Go and Python scrapers
see byte-identical payloads. Throwaway debugging tool — not wired into CI.
"""

import json
import sys
import traceback

from datadog_checks.base.checks.openmetrics.v2.base import OpenMetricsBaseCheckV2

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


for _kind in ("gauge", "count", "rate", "monotonic_count", "histogram", "historate"):
    setattr(OpenMetricsBaseCheckV2, _kind, _mk(_kind))


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


def _run_one(req: dict) -> dict:
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
        resp = _run_one(req)
        print(json.dumps(resp, default=str), flush=True)


if __name__ == "__main__":
    main()
