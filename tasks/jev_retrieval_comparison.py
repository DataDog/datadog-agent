"""One bounded live Stage 9 pair; private artifacts, no prediction or publication.

Run through the normal Jev `dda inv` runtime using the `qualify` task below.
The coordinator prepares frozen inputs, then starts a fresh process per path. It
analyzes full response captures only after the measured workers have exited.
"""

from __future__ import annotations

import argparse
import contextlib
import dataclasses
import hashlib
import importlib
import importlib.metadata
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import sys
import time
import traceback
import types
from collections import Counter
from datetime import datetime, timedelta, timezone
from unittest.mock import patch
from urllib.parse import urlsplit

from invoke import Exit, task


BASELINE = "8db4d8733c4484b7a07ccd6d0a21b564322b83fe"
REVISION = "9aa8114582dc5e0530a24c3c4bbd575ded8ab9d0"
PIPELINE = "148476892"
HELPER = "tasks/libs/common/datadog_api.py"
EVALUATOR = "tasks/libs/dynamic_test/evaluator.py"
BASELINE_HASHES = {
    HELPER: "663af5a2a95f18ea8d89028e0f5422c53235777d71a24dc7250012fa3e1128da",
    EVALUATOR: "fcfdee65cc75a3516f3a5a140ec1a5d2d6b8c7073a82352a79314b1e13c18715",
}
DEPENDENCY_HASHES = {
    "tasks/libs/dynamic_test/index.py": "b94efd1758186aae8c2bb84e736032da2bfb983a225aad2bde41d56be0db1bb2",
    "tasks/libs/dynamic_test/executor.py": "8a0e2dd7481010283df421867fd0d59b6742aefe069fc311dedf0e92b80f62f5",
    "tasks/libs/dynamic_test/telemetry.py": "eb78d4adeca4c24c26f139afc35539c062c9d4e3230d8fdf95a1cdaae082aa6b",
    "tasks/libs/dynamic_test/jev_selection.py": "1fef752c9fb778aa760d5b17806463fa5ea5f8fc8602dbd369c600f1af10920f",
}
END = datetime(2026, 10, 30, tzinfo=timezone.utc)
START = END - timedelta(days=3)
KINDS = {"jev", "package", "file", "diffed_package"}
ANSI = re.compile(r"\x1b\[[0-?]*[ -/]*[@-~]")
HOST = "api.datadoghq.com"


def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate_launcher():
    require(os.environ.get("JEV_COMPARISON_LAUNCHER") == "dda inv",
            "Qualification must enter through the normal dda inv runtime")
    require(os.environ.get("JEV_COMPARISON_LAUNCHER_PYTHON") == sys.executable,
            "Qualification interpreter differs from the normal Invoke launcher")


def sha(data):
    return hashlib.sha256(data).hexdigest()


def encoded(value, *, canonical=False):
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=canonical).encode("utf-8")


def write_json(path, value):
    with path.open("x", encoding="utf-8", newline="\n") as stream:
        json.dump(value, stream, ensure_ascii=False, indent=2)
        stream.write("\n")


def now():
    return datetime.now(timezone.utc).isoformat()


def parse_time(value):
    result = datetime.fromisoformat(str(value).replace("Z", "+00:00"))
    return result.replace(tzinfo=timezone.utc) if result.tzinfo is None else result.astimezone(timezone.utc)


def safe_error(error):
    # SDK exception strings contain HTTP headers. Preserve type, status, and
    # stack locations; full HTTP response bodies are retained separately.
    return {
        "type": f"{type(error).__module__}.{type(error).__name__}",
        "status": getattr(error, "status", None),
        "reason_type": type(getattr(error, "reason", None)).__name__,
        "frames": [{"file": row.filename, "line": row.lineno, "function": row.name}
                   for row in traceback.extract_tb(error.__traceback__)],
    }


def validate_protocol(document):
    require(document.get("schema_version") == 1, "Unsupported protocol version")
    require(document.get("baseline_revision") == BASELINE, "Baseline revision mismatch")
    require(str(document.get("pipeline_id")) == PIPELINE, "Document a replacement case before changing pipeline")
    require(document.get("evaluation_revision") == REVISION, "Evaluation revision mismatch")
    window = document.get("window", {})
    require(parse_time(window.get("from")) == START and parse_time(window.get("to")) == END,
            "The absolute three-day UTC window differs")
    hashes = document.get("source_sha256", {})
    require(hashes.get("baseline") == BASELINE_HASHES, "Stage 8 baseline hash binding differs")
    require(hashes.get("dependencies") == DEPENDENCY_HASHES, "Stage 8 dependency hash binding differs")
    require(set(hashes.get("candidate", {})) == {HELPER, EVALUATOR}, "Candidate source binding is incomplete")
    require(all(re.fullmatch(r"[a-f0-9]{64}", value) for value in hashes["candidate"].values()),
            "Candidate hashes are invalid")
    require(document.get("run_order") in (["baseline", "candidate"], ["candidate", "baseline"]),
            "Exactly one ordered baseline/candidate pair is required")
    allowed = document.get("allowed_failure", {})
    require(str(allowed.get("pipeline_id")) == PIPELINE, "Allowed-failure pipeline mismatch")
    if allowed.get("source") != "ci_gitlab":
        require(isinstance(allowed.get("jobs"), list) and isinstance(allowed.get("provenance"), dict)
                and allowed["provenance"], "Frozen actual allowed-failure state and provenance are required")
        require(all(isinstance(job, str) for job in allowed["jobs"])
                and len(set(allowed["jobs"])) == len(allowed["jobs"]), "Allowed-failure jobs are invalid")
    cases = document.get("cases")
    require(isinstance(cases, list) and 1 <= len(cases) <= 4, "One to four bound mode cases are required")
    require(len({case["name"] for case in cases}) == len(cases), "Case names must be unique")
    require(len({case["kind"] for case in cases}) == len(cases), "Each mode may be measured only once")
    for case in cases:
        require(re.fullmatch(r"[a-z0-9_-]+", case["name"]) and case["kind"] in KINDS, "Invalid case name or kind")
        index, predictions = case.get("index"), case.get("predictions")
        require(isinstance(index, dict) and index and isinstance(predictions, dict), "Case inputs are missing")
        require(list(index) == list(predictions), "Predictions must cover the complete ordered index")
        require(isinstance(case.get("provenance"), dict) and case["provenance"], "Case provenance is required")
        for job, tests in index.items():
            require(isinstance(job, str) and job and isinstance(tests, list)
                    and all(isinstance(test, str) for test in tests), "Invalid job index")
            require(len(tests) == len(set(tests)), "Duplicate index tests would be hidden by the adapter")
            predicted = predictions[job]
            require(isinstance(predicted, list) and all(isinstance(test, str) for test in predicted)
                    and len(predicted) == len(set(predicted)), "Invalid retained predictions")
            require(set(predicted) <= set(tests), "Retained predictions lie outside the bound index")
        if case["kind"] == "jev":
            require(len(index) == 322, "The selected Jev index must contain all 322 jobs")
    require(any(case["kind"] == "jev" for case in cases), "The selected Jev case is required")
    require(isinstance(document.get("ci_configuration"), dict) and document["ci_configuration"],
            "Bind the effective nonsecret CI configuration")


def git_bytes(source, *arguments):
    result = subprocess.run(["git", "-C", str(source), *arguments], stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, check=False)
    require(result.returncode == 0, "Required local Git source is unavailable; no automatic fetch is performed")
    return result.stdout


def retain_sources(source, document, out):
    result = {"head": git_bytes(source, "rev-parse", "HEAD").decode().strip(), "files": {}}
    for variant in ("baseline", "candidate", "dependencies"):
        for name, expected in document["source_sha256"][variant].items():
            data = git_bytes(source, "show", f"{BASELINE}:{name}") if variant == "baseline" else (source / name).read_bytes()
            require(sha(data) == expected, f"Source hash mismatch: {variant}/{name}")
            target = out / "sources" / variant / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(data)
            result["files"][f"{variant}/{name}"] = sha(data)
    return result


def prepare_allowed_failure(document, source):
    if document["allowed_failure"].get("source") != "ci_gitlab":
        return
    sys.path.insert(0, str(source))
    from tasks.libs.ciproviders.gitlab_api import get_pipeline

    pipeline = get_pipeline("DataDog/datadog-agent", document["pipeline_id"])
    require(str(pipeline.id) == PIPELINE and pipeline.sha == REVISION, "GitLab pipeline identity mismatch")
    require(pipeline.status in {"success", "failed", "canceled", "skipped"}
            and getattr(pipeline, "finished_at", None), "The selected pipeline must have finished")
    records = []
    # Match production's two terminal-status queries and complete pagination.
    for scope in ("success", "failed"):
        for job in pipeline.jobs.list(scope=scope, iterator=True):
            records.append({"scope": scope, "id": job.id, "name": job.name, "status": job.status,
                            "allow_failure": getattr(job, "allow_failure", False),
                            "retried": getattr(job, "retried", None),
                            "started_at": getattr(job, "started_at", None),
                            "finished_at": getattr(job, "finished_at", None)})
    document["allowed_failure"] = {
        "pipeline_id": PIPELINE, "source": "frozen_ci_gitlab",
        "jobs": sorted({row["name"] for row in records if row["allow_failure"]}),
        "provenance": {"captured_at": now(), "pipeline": {
            key: getattr(pipeline, key, None) for key in ("id", "sha", "status", "finished_at", "web_url")},
            "contract": "success and failed scopes; iterator=True; same as pipeline_jobs_allowed_to_fail",
            "job_records": records},
    }


class FrozenDatetime(datetime):
    @classmethod
    def now(cls, tz=None):
        return END.replace(tzinfo=None) if tz is None else END.astimezone(tz)


class FrozenIndex:
    def __init__(self, index):
        self.data = {job: set(tests) for job, tests in index.items()}

    def to_dict(self):
        return self.data

    def get_indexed_tests_for_job(self, job):
        return self.data[job]


class RecordingTelemetry:
    def __init__(self):
        self.payloads = []

    def send_event(self, event):
        self.payloads.append({"method": "send_event", "event": dataclasses.asdict(event)})
        return True

    def count(self, name, value=1, tags=None):
        self.payloads.append({"method": "count", "name": name, "value": value, "tags": tags})
        return True


def load_module(name, data, filename):
    module = types.ModuleType(name)
    module.__file__ = filename
    sys.modules[name] = module
    exec(compile(data, filename, "exec"), module.__dict__)
    return module


def production_modules(source, document, variant, record):
    sys.path.insert(0, str(source))
    helper = importlib.import_module("tasks.libs.common.datadog_api")
    evaluator = importlib.import_module("tasks.libs.dynamic_test.evaluator")
    for name, module in ((HELPER, helper), (EVALUATOR, evaluator)):
        require(Path(module.__file__).resolve() == (source / name).resolve(), "Imported production module is outside checkout")
        require(sha((source / name).read_bytes()) == document["source_sha256"]["candidate"][name], "Candidate source drift")
    for name, expected in DEPENDENCY_HASHES.items():
        require(sha((source / name).read_bytes()) == expected, "Dependency source drift")
    if variant == "baseline":
        blobs = {name: (record / "sources/baseline" / name).read_bytes() for name in BASELINE_HASHES}
        require(all(sha(blobs[name]) == expected for name, expected in BASELINE_HASHES.items()), "Baseline source drift")
        helper = load_module("_stage9_baseline_helper", blobs[HELPER], f"git:{BASELINE}:{HELPER}")
        evaluator = load_module("_stage9_baseline_evaluator", blobs[EVALUATOR], f"git:{BASELINE}:{EVALUATOR}")
        evaluator.get_ci_test_events = helper.get_ci_test_events
    return evaluator, helper


def memory():
    import resource

    rss = None
    status = Path("/proc/self/status")
    if status.exists():
        for line in status.read_text().splitlines():
            if line.startswith("VmRSS:"):
                rss = int(line.split()[1]) * 1024
    return {"rss_bytes": rss, "ru_maxrss_bytes": resource.getrusage(resource.RUSAGE_SELF).ru_maxrss * 1024}


def normalized_summary(text):
    lines = ANSI.sub("", text).splitlines(keepends=True)
    offset = 0
    while offset < len(lines):
        if lines[offset].startswith("WARNING: The following failing tests would not have been executed:"):
            end = offset + 1
            while end < len(lines) and lines[end].startswith("- "):
                end += 1
            lines[offset + 1:end] = sorted(lines[offset + 1:end])
            offset = end
        else:
            offset += 1
    return "".join(lines)


def check_target(method, url):
    parsed = urlsplit(url)
    paths = {"GET": "/api/v2/ci/tests/events", "POST": "/api/v2/ci/tests/events/search"}
    require(parsed.scheme == "https" and parsed.hostname == HOST and parsed.port in (None, 443)
            and parsed.username is None and parsed.password is None and parsed.path == paths.get(method),
            "Only the existing CI Visibility read endpoints on api.datadoghq.com are allowed")


def request_spec(variant, method, url, kwargs, query_jobs):
    check_target(method, url)
    require(not urlsplit(url).query and kwargs.get("preload_content") is True, "Unexpected request transport form")
    require(method == ("GET" if variant == "baseline" else "POST"), "Production HTTP operation changed")
    if method == "GET":
        pairs = kwargs["fields"]
        params = dict(pairs)
        require(len(params) == len(pairs), "Duplicate GET parameters")
        require(set(params) <= {"filter[query]", "filter[from]", "filter[to]", "page[limit]", "page[cursor]"},
                "Baseline must retain its unspecified sort and original query parameters")
        query, start, end = params["filter[query]"], params["filter[from]"], params["filter[to]"]
        cursor, limit = params.get("page[cursor]"), params["page[limit]"]
        sort = None
    else:
        body = json.loads(kwargs["body"])
        require(set(body) == {"filter", "page", "sort"} and body["sort"] == "-timestamp", "Candidate POST shape changed")
        require(set(body["filter"]) == {"query", "from", "to"}
                and set(body["page"]) <= {"limit", "cursor"}, "Candidate query fields changed")
        query, start, end = body["filter"]["query"], body["filter"]["from"], body["filter"]["to"]
        cursor, limit, sort = body["page"].get("cursor"), body["page"]["limit"], body["sort"]
    require(parse_time(start) == START and parse_time(end) == END and limit == 1000, "Request bounds or page limit changed")
    require(query in query_jobs, "Request does not match the complete bound production query scope")
    return {"method": method, "query": query, "query_sha256": sha(query.encode()), "jobs": query_jobs[query],
            "cursor": cursor, "sort": sort, "from": str(start), "to": str(end), "limit": limit}


class LiveObserver:
    def __init__(self, variant, query_jobs, out):
        self.variant, self.query_jobs, self.out = variant, query_jobs, out
        self.pages, self.retries, self.sleeps, self.decodes = [], [], [], []
        self.active_page = None
        self.capture_s = 0.0
        self.request_observer_s = 0.0
        self.retrieval_s = 0.0
        self.retrieval_calls = 0
        self.client_count = 0
        self.client_close_count = 0
        self.client_init_s = 0.0
        self.client_close_s = 0.0

    def capture(self, data, name):
        started = time.perf_counter()
        path = self.out / name
        with path.open("xb") as stream:
            stream.write(data)
        result = {"file": name, "bytes": len(data), "sha256": sha(data)}
        self.capture_s += time.perf_counter() - started
        return result

    @contextlib.contextmanager
    def installed(self):
        from datadog_api_client import ApiClient
        from urllib3 import PoolManager
        from urllib3.util.retry import Retry

        original_request, original_urlopen = PoolManager.request, PoolManager.urlopen
        original_decode = ApiClient.deserialize
        original_init, original_close = ApiClient.__init__, ApiClient.close
        original_increment, original_sleep = Retry.increment, Retry.sleep

        def request(pool, method, url, **kwargs):
            observed = time.perf_counter()
            row = request_spec(self.variant, method, url, kwargs, self.query_jobs)
            row.update(number=len(self.pages) + 1, started_at=now())
            self.pages.append(row)
            self.active_page = row["number"]
            before_call = time.perf_counter()
            self.request_observer_s += before_call - observed
            try:
                response = original_request(pool, method, url, **kwargs)
                row["http_status"] = response.status
            except BaseException as error:
                row["error"] = safe_error(error)
                raise
            finally:
                row["pool_request_s"] = time.perf_counter() - before_call
                self.active_page = None
            observed = time.perf_counter()
            row["response"] = self.capture(response.data, f"page-{row['number']:05d}.body")
            history = getattr(getattr(response, "retries", None), "history", ())
            row["terminal_retry_history"] = [{"method": item.method, "status": item.status,
                                                "error_type": type(item.error).__name__ if item.error else None,
                                                "redirect_present": bool(item.redirect_location)} for item in history]
            row["completed_at"] = now()
            self.request_observer_s += time.perf_counter() - observed
            return response

        def urlopen(pool, method, url, *args, **kwargs):
            # PoolManager follows redirects inside urlopen, below request().
            check_target(method, url)
            return original_urlopen(pool, method, url, *args, **kwargs)

        def decode(client, *args, **kwargs):
            started = time.perf_counter()
            row = {"page": len(self.pages)}
            try:
                return original_decode(client, *args, **kwargs)
            except BaseException as error:
                row["error"] = safe_error(error)
                raise
            finally:
                row["elapsed_s"] = time.perf_counter() - started
                self.decodes.append(row)

        def initialize(client, *args, **kwargs):
            started = time.perf_counter()
            try:
                return original_init(client, *args, **kwargs)
            finally:
                self.client_count += 1
                self.client_init_s += time.perf_counter() - started

        def close(client, *args, **kwargs):
            started = time.perf_counter()
            try:
                return original_close(client, *args, **kwargs)
            finally:
                self.client_close_count += 1
                self.client_close_s += time.perf_counter() - started

        def increment(retry, method=None, url=None, response=None, error=None, *args, **kwargs):
            row = {"page": self.active_page, "method": method,
                   "status": getattr(response, "status", None), "error_type": type(error).__name__ if error else None,
                   "budget_before": retry.total}
            self.retries.append(row)
            # Read already preloaded bytes only; do not force a response read
            # that the production retry path would otherwise avoid.
            data = getattr(response, "_body", None)
            if isinstance(data, bytes):
                row["response"] = self.capture(data, f"retry-{len(self.retries):05d}.body")
            else:
                row["response_body_unavailable_without_forcing_read"] = True
            try:
                result = original_increment(retry, method, url, response, error, *args, **kwargs)
                row["budget_after"] = result.total
                return result
            except BaseException as failure:
                row["increment_error"] = safe_error(failure)
                raise

        def sleep(retry, response=None):
            started = time.perf_counter()
            try:
                return original_sleep(retry, response)
            finally:
                self.sleeps.append({"page": self.active_page, "elapsed_s": time.perf_counter() - started})

        with contextlib.ExitStack() as stack:
            for owner, name, replacement in ((PoolManager, "request", request), (PoolManager, "urlopen", urlopen),
                                              (ApiClient, "deserialize", decode), (ApiClient, "__init__", initialize),
                                              (ApiClient, "close", close), (Retry, "increment", increment), (Retry, "sleep", sleep)):
                stack.enter_context(patch.object(owner, name, replacement))
            yield

    def ordinary_retrieval(self, original):
        def wrapped(*args, **kwargs):
            started = time.perf_counter()
            try:
                return original(*args, **kwargs)
            finally:
                self.retrieval_calls += 1
                self.retrieval_s += time.perf_counter() - started
        return wrapped

    def streamed_retrieval(self, original):
        def wrapped(*args, **kwargs):
            started = time.perf_counter()
            iterator = original(*args, **kwargs)
            self.retrieval_calls += 1
            self.retrieval_s += time.perf_counter() - started
            try:
                while True:
                    started = time.perf_counter()
                    try:
                        event = next(iterator)
                    except StopIteration:
                        return
                    finally:
                        self.retrieval_s += time.perf_counter() - started
                    yield event
            finally:
                started = time.perf_counter()
                try:
                    iterator.close()
                finally:
                    self.retrieval_s += time.perf_counter() - started
        return wrapped


def measurement_totals(observer):
    return {
        "retrieval_gross_s": observer.retrieval_s,
        "retrieval_calls": observer.retrieval_calls,
        "pool_requests_including_retries_and_backoff_s": sum(row["pool_request_s"] for row in observer.pages),
        "sdk_deserialization_s": sum(row["elapsed_s"] for row in observer.decodes),
        "raw_capture_write_and_hash_s": observer.capture_s,
        "request_observer_outside_pool_s": observer.request_observer_s,
        "retry_sleep_observed_s": sum(row["elapsed_s"] for row in observer.sleeps),
        "page_calls": len(observer.pages),
        "successful_page_calls": sum(row.get("http_status") == 200 for row in observer.pages),
        "retry_increment_calls": len(observer.retries),
        "response_bytes": sum(row.get("response", {}).get("bytes", 0) for row in observer.pages),
        "sdk_client_initializations": observer.client_count,
        "sdk_client_close_calls": observer.client_close_count,
        "sdk_client_initialization_s": observer.client_init_s,
        "sdk_client_close_s": observer.client_close_s,
    }


def worker(args):
    started = time.perf_counter()
    out = args.out.resolve()
    document = json.loads(args.input.read_bytes())
    validate_protocol(document)
    require(sys.platform.startswith("linux"), "The paired live probe requires the existing Linux CI runtime")
    require(os.environ.get("CI") == "true", "Live qualification is CI-only")
    validate_launcher()
    require(os.environ.get("DD_SITE", "datadoghq.com") == "datadoghq.com", "Existing CI site differs")
    require(os.environ.get("DD_API_KEY") and os.environ.get("DD_APP_KEY"), "Existing CI credentials are unavailable")
    os.environ["TZ"] = "UTC"
    time.tzset()
    case = next(case for case in document["cases"] if case["name"] == args.case)
    module, helper = production_modules(args.source.resolve(), document, args.worker, out.parent)
    from datadog_api_client import Configuration
    from invoke import Context

    configuration = Configuration(enable_retry=True)
    require(not configuration.proxy, "A proxy would change the inspected direct transport path")
    require(not configuration.debug, "SDK debug logging must remain disabled to protect request credentials")
    require(getattr(configuration, "retry_policy", None) is None, "A custom retry policy changes the protocol")
    runtime = {"python": sys.version, "executable": sys.executable, "prefix": sys.prefix, "base_prefix": sys.base_prefix,
               "platform": platform.platform(),
               "architecture": platform.machine(), "packages": {
                   name: importlib.metadata.version(name) for name in ("datadog-api-client", "urllib3", "invoke")},
               "sdk_retry_configuration": {name: getattr(configuration, name) for name in (
                   "enable_retry", "max_retries", "retry_backoff_factor")},
               "sdk_sources": {}}
    for name in ("datadog_api_client.api_client", "datadog_api_client.rest", "datadog_api_client.model_utils",
                 "datadog_api_client.v2.api.ci_visibility_tests_api", "urllib3.connectionpool", "urllib3.util.retry"):
        path = Path(importlib.import_module(name).__file__)
        runtime["sdk_sources"][name] = {"path": str(path), "sha256": sha(path.read_bytes())}
    index = FrozenIndex(case["index"])
    predictions = {job: set(tests) for job, tests in case["predictions"].items()}
    prediction_measurement = {"calls": 0, "elapsed_s": 0.0, "provider_calls": 0}

    def predict(changes):
        before = time.perf_counter()
        require(changes == [], "Frozen retained prediction adapter received unexpected changes")
        prediction_measurement["calls"] += 1
        prediction_measurement["elapsed_s"] += time.perf_counter() - before
        return predictions

    executor = types.SimpleNamespace(commit_sha=REVISION, init_index=lambda: None,
                                     index=lambda: index, tests_to_run_per_job=predict)
    telemetry = RecordingTelemetry()
    evaluator = module.DatadogDynTestEvaluator(Context(), module.IndexKind(case["kind"]), executor, PIPELINE, telemetry)
    query_jobs = {}
    if args.worker == "baseline":
        for job in case["index"]:
            escaped = job.replace('"', '\\"')
            query_jobs[f'env:prod @ci.pipeline.name:DataDog/datadog-agent @ci.pipeline.id:{PIPELINE} '
                       f'@ci.job.name:"{escaped}" -@test.status:skip'] = [job]
    else:
        for query in evaluator._test_event_queries(list(case["index"])):
            names = [json.loads(value) for value in re.findall(r'@ci\.job\.name:("(?:[^"\\]|\\.)*")', query)]
            query_jobs[query] = names
        require([job for jobs in query_jobs.values() for job in jobs] == list(case["index"]), "Candidate query partition differs")
    observer = LiveObserver(args.worker, query_jobs, out)
    report = {"variant": args.worker, "case": args.case, "status": "running", "started_at": now(),
              "runtime": runtime, "prediction": prediction_measurement, "memory_before_evaluate": memory(),
              "measurements": {}, "output": {}}
    evaluation_job_s = 0.0
    reduction_capture_s = 0.0
    original_job = evaluator._evaluate_job
    with (out / "reduced.jsonl").open("x", encoding="utf-8", newline="\n") as reductions:
        def evaluate_job(job, tests, predicted):
            nonlocal evaluation_job_s, reduction_capture_s
            before = time.perf_counter()
            reductions.write(encoded({"job_name": job, "tests": [dataclasses.asdict(test) for test in tests]}).decode() + "\n")
            reduction_capture_s += time.perf_counter() - before
            before = time.perf_counter()
            try:
                return original_job(job, tests, predicted)
            finally:
                evaluation_job_s += time.perf_counter() - before

        try:
            with contextlib.ExitStack() as stack:
                stack.enter_context(observer.installed())
                stack.enter_context(patch.object(helper, "datetime", FrozenDatetime))
                if hasattr(module, "datetime"):
                    stack.enter_context(patch.object(module, "datetime", FrozenDatetime))
                stack.enter_context(patch.object(module, "pipeline_jobs_allowed_to_fail", lambda pipeline_id: set(document["allowed_failure"]["jobs"])))
                telemetry_module = importlib.import_module("tasks.libs.dynamic_test.telemetry")
                # Replace only this module's reference; never freeze urllib3's clock.
                stack.enter_context(patch.object(telemetry_module, "time", types.SimpleNamespace(time=lambda: END.timestamp())))
                stack.enter_context(patch.object(evaluator, "_evaluate_job", evaluate_job))
                retrieval_name = "get_ci_test_events" if args.worker == "baseline" else "iter_ci_test_events"
                original = getattr(module, retrieval_name)
                wrapper = observer.ordinary_retrieval if args.worker == "baseline" else observer.streamed_retrieval
                stack.enter_context(patch.object(module, retrieval_name, wrapper(original)))
                with (out / "diagnostics.txt").open("x", encoding="utf-8", newline="\n") as diagnostics:
                    with contextlib.redirect_stdout(diagnostics):
                        require(evaluator.initialize(), "Production evaluator initialization failed")
                        before = time.perf_counter()
                        try:
                            results = evaluator.evaluate([])
                        finally:
                            report["measurements"]["evaluate_gross_s"] = time.perf_counter() - before
                            report["memory_after_evaluate"] = memory()
                before = time.perf_counter()
                with (out / "summary.txt").open("x", encoding="utf-8", newline="\n") as summary:
                    with contextlib.redirect_stdout(summary):
                        evaluator.print_summary(results)
                evaluator.send_stats_to_datadog(results)
                report["measurements"]["summary_and_isolated_telemetry_s"] = time.perf_counter() - before
                report["output"]["results"] = [{
                    "job_name": result.job_name, "actual_executed_tests": sorted(result.actual_executed_tests),
                    "predicted_executed_tests": sorted(result.predicted_executed_tests),
                    "not_executed_failing_tests": sorted(result.not_executed_failing_tests)} for result in results]
                report["output"]["global_counts"] = {
                    "actual": sum(result.actual_count() for result in results),
                    "predicted": sum(result.predicted_count() for result in results),
                    "not_executed_failing": sum(result.not_executed_failing_count() for result in results)}
                require(prediction_measurement["calls"] == 1, "Predictions must be obtained exactly once per path from the frozen adapter")
                report["status"] = "completed"
        except BaseException as error:
            report["status"] = "failed"
            report["error"] = safe_error(error)
    report["output"]["telemetry_payloads"] = telemetry.payloads
    report["measurements"].update(measurement_totals(observer))
    report["measurements"].update(evaluate_job_s=evaluation_job_s, reduction_capture_serialization_s=reduction_capture_s)
    report["transport"] = {"query_jobs": query_jobs, "pages": observer.pages, "retries": observer.retries,
                           "retry_sleeps": observer.sleeps, "sdk_decodes": observer.decodes}
    report["measurements"]["worker_to_report_serialization_s"] = time.perf_counter() - started
    report["memory_before_report_serialization"] = memory()
    report["completed_at"] = now()
    write_json(out / "result.json", report)
    write_json(out / "process-end.json", {"worker_through_report_write_s": time.perf_counter() - started,
                                          "memory": memory()})
    return 0 if report["status"] == "completed" else 1


def analyze_events(out, result, case):
    identities, bodies = Counter(), Counter()
    by_job = {job: [] for job in case["index"]}
    invalid = []
    incomplete = []
    query_progress = {}
    seen_cursors = {}
    for page in result.get("transport", {}).get("pages", []):
        query = page["query"]
        if page.get("cursor") != query_progress.get(query):
            incomplete.append({"page": page["number"], "reason": "request cursor does not follow captured response"})
        if "response" not in page:
            incomplete.append({"page": page["number"], "reason": "no response body"})
            continue
        data = (out / page["response"]["file"]).read_bytes()
        require(sha(data) == page["response"]["sha256"], "Response capture hash mismatch")
        try:
            response = json.loads(data)
        except (ValueError, UnicodeError):
            incomplete.append({"page": page["number"], "reason": "non-JSON response"})
            continue
        if not isinstance(response, dict) or not isinstance(response.get("data"), list) or page.get("http_status") != 200:
            incomplete.append({"page": page["number"], "reason": "missing data or unsuccessful HTTP status"})
            continue
        metadata = response.get("meta")
        if metadata is not None and (not isinstance(metadata, dict) or metadata.get("status") not in (None, "done") or metadata.get("warnings")):
            incomplete.append({"page": page["number"], "reason": "response status or warnings indicate incompleteness"})
        pagination = metadata.get("page") if isinstance(metadata, dict) else None
        cursor = pagination.get("after") if isinstance(pagination, dict) else None
        links = response.get("links")
        if ((pagination is not None and not isinstance(pagination, dict))
                or (cursor is not None and not isinstance(cursor, str))
                or (links is not None and not isinstance(links, dict))):
            incomplete.append({"page": page["number"], "reason": "malformed pagination or links"})
            cursor = None
        if isinstance(links, dict) and links.get("next") and not cursor:
            incomplete.append({"page": page["number"], "reason": "advertised next link without usable cursor"})
        if cursor and cursor in seen_cursors.setdefault(query, set()):
            incomplete.append({"page": page["number"], "reason": "repeated response cursor"})
        if cursor:
            seen_cursors[query].add(cursor)
        query_progress[query] = cursor or None
        for event in response["data"]:
            if not isinstance(event, dict):
                invalid.append({"page": page["number"], "reason": "non-object event"})
                continue
            attrs = event.get("attributes", {}).get("attributes", {})
            ci = attrs.get("ci", {})
            job = ci.get("job", {}).get("name")
            fingerprint = sha(encoded(event, canonical=True))
            identity = encoded({"id": event.get("id"), "type": event.get("type")}, canonical=True).decode()
            identities[identity] += 1
            bodies[fingerprint] += 1
            if job not in by_job or str(ci.get("pipeline", {}).get("id")) != PIPELINE or attrs.get("env") != "prod" or attrs.get("test", {}).get("status") == "skip":
                invalid.append({"page": page["number"], "identity": identity, "reason": "event outside bound predicates"})
                continue
            if not event.get("id"):
                invalid.append({"page": page["number"], "reason": "missing event identity"})
            by_job[job].append({"identity": identity, "event_sha256": fingerprint})
    for query, cursor in query_progress.items():
        if cursor:
            incomplete.append({"query_sha256": sha(query.encode()), "reason": "unconsumed continuation cursor"})
    expected = set(result.get("transport", {}).get("query_jobs", {}))
    if set(query_progress) != expected:
        incomplete.append({"reason": "captured queries do not cover every bound production query"})
    return {"event_count": sum(identities.values()), "identity_multiplicity": dict(sorted(identities.items())),
            "full_event_multiplicity": dict(sorted(bodies.items())), "per_job_order": by_job,
            "invalid_events": invalid, "incomplete_responses": incomplete}


def parity_checks(left, right):
    checks = {"both_completed": left["status"] == right["status"] == "completed"}
    for field in ("identity_multiplicity", "full_event_multiplicity", "per_job_order"):
        checks[field] = left["events"][field] == right["events"][field]
    for field in ("results", "global_counts", "telemetry_payloads"):
        checks[field] = field in left["output"] and field in right["output"] and left["output"][field] == right["output"][field]
    for field in ("reduced", "diagnostics", "summary"):
        checks[field] = field in left and field in right and left[field] == right[field]
    checks["nonempty_event_population"] = left["events"]["event_count"] > 0 and right["events"]["event_count"] > 0
    checks["complete_valid_responses"] = not any(
        item["events"][key] for item in (left, right) for key in ("invalid_events", "incomplete_responses"))
    return {"exact_checks": checks, "exact_pass": all(checks.values()),
            "supplementary_warning_set_normalized_summary_equal": normalized_summary(left.get("summary", "")) == normalized_summary(right.get("summary", ""))}


def coordinator(args):
    started = time.perf_counter()
    source = args.source.resolve()
    out = args.out.resolve()
    require(not out.exists(), "Output directory must be new; failed attempts must be preserved")
    os.umask(0o077)
    out.mkdir(parents=True, mode=0o700)
    document = json.loads(args.input.read_bytes())
    report = {"status": "preparing", "started_at": now(), "input_sha256": sha(args.input.read_bytes()),
              "script_sha256": sha(Path(__file__).read_bytes()), "run_order": document.get("run_order"),
              "source": str(source), "cases": {}, "workers": [], "limitations": [
                  "Fixed bounds are not an atomic snapshot; differences can reflect ingestion or unspecified baseline sorting and timestamp ties.",
                  "Retrieval is gross execution time in complete production helpers, including SDK/client lifetimes, decode and synchronous capture overhead.",
                  "Candidate retrieval sums generator execution slices; consumer reduction runs while the generator is suspended.",
                  "Decode, client lifecycle, transport and capture measurements are nested and must not be added to retrieval or evaluate totals.",
                  "raw_capture_write_and_hash_s is nested in request observer time for final pages and transport time for retry bodies. Observer overhead also includes unisolated timer/wrapper bookkeeping.",
                  "Full process RSS/ru_maxrss includes imports, input adapters, SDK caches, result storage, and instrumentation; it is not a production-only memory delta.",
                  "Raw response bodies contain private events and stay in restricted artifacts; credentials and request/response headers are never recorded.",
                  "Retry increment events and terminal histories are observed, not inferred from max_retries. A final non-retried error has no retry increment.",
                  "Exact parity remains primary. Only an additional warning-set summary comparison normalizes the preexisting unordered warning set.",
                  "Whole probe time is qualification time; it is not normal job runtime or required pre-queue readiness.",
              ]}
    try:
        validate_protocol(document)
        require(sys.platform.startswith("linux") and os.environ.get("CI") == "true", "Live qualification is restricted to the existing Linux CI runtime")
        validate_launcher()
        report["sources"] = retain_sources(source, document, out)
        before = time.perf_counter()
        with (out / "preparation.log").open("x", encoding="utf-8") as log:
            with contextlib.redirect_stdout(log), contextlib.redirect_stderr(log):
                prepare_allowed_failure(document, source)
        report["allowed_failure_preparation_s"] = time.perf_counter() - before
        validate_protocol(document)
        report["ci_configuration"] = document["ci_configuration"]
        report["ci_runtime_identifiers"] = {name: os.environ.get(name) for name in (
            "CI_JOB_ID", "CI_JOB_URL", "CI_PIPELINE_ID", "CI_PIPELINE_URL", "CI_COMMIT_SHA", "CI_JOB_IMAGE",
            "CI_RUNNER_ID", "CI_RUNNER_DESCRIPTION", "CI_RUNNER_EXECUTABLE_ARCH", "CI_IMAGE_LINUX", "CI_IMAGE_LINUX_SUFFIX",
            "DDA_NO_DYNAMIC_DEPS", "JEV_COMPARISON_LAUNCHER", "JEV_COMPARISON_LAUNCHER_PYTHON")}
        report["coverage"] = {kind: ("paired live" if any(case["kind"] == kind for case in document["cases"])
                                     else "Stage 8 synthetic tests only; no bound retained live inputs supplied") for kind in sorted(KINDS)}
        prepared = out / "prepared-input.json"
        write_json(prepared, document)
        report["prepared_input_sha256"] = sha(prepared.read_bytes())
        for case in document["cases"]:
            for variant in document["run_order"]:
                path = out / f"{case['name']}-{variant}"
                path.mkdir(mode=0o700)
                command = [sys.executable, "-B", str(Path(__file__).resolve()), "--input", str(prepared),
                           "--source", str(source), "--out", str(path), "--worker", variant, "--case", case["name"]]
                before = time.perf_counter()
                with (path / "worker-console.log").open("xb") as log:
                    # A shared seed removes cross-process hash randomization as
                    # a confounder; production iteration and raw text stay intact.
                    environment = dict(os.environ, PYTHONHASHSEED="0", PYTHONDONTWRITEBYTECODE="1", TZ="UTC")
                    process = subprocess.run(command, cwd=source, env=environment, stdout=log, stderr=log, check=False)
                report["workers"].append({"case": case["name"], "variant": variant, "returncode": process.returncode,
                                          "process_wall_s": time.perf_counter() - before,
                                          "result_present": (path / "result.json").exists()})
        # Analyze after all measured children have exited: parsing raw captures
        # must not contribute to either path's RSS or compete with live work.
        for case in document["cases"]:
            variants = {}
            for variant in document["run_order"]:
                path = out / f"{case['name']}-{variant}"
                result = json.loads((path / "result.json").read_bytes()) if (path / "result.json").exists() else {
                    "status": "worker_failed_without_result", "output": {}}
                events = analyze_events(path, result, case)
                write_json(path / "events-analysis.json", events)
                result["events"] = events
                for field, filename in (("reduced", "reduced.jsonl"), ("diagnostics", "diagnostics.txt"), ("summary", "summary.txt")):
                    if (path / filename).exists():
                        value = (path / filename).read_text(encoding="utf-8")
                        result[field] = [json.loads(line) for line in value.splitlines()] if field == "reduced" else value
                variants[variant] = result
            checks = parity_checks(variants["baseline"], variants["candidate"])
            report["cases"][case["name"]] = {"kind": case["kind"], "jobs": len(case["index"]),
                "index_sha256": sha(encoded(case["index"])), "predictions_sha256": sha(encoded(case["predictions"])),
                "parity": checks, "measurements": {variant: variants[variant].get("measurements", {}) for variant in variants}}
        report["status"] = "passed" if all(row["parity"]["exact_pass"] for row in report["cases"].values()) else "requires_investigation"
    except BaseException as error:
        report["status"] = "failed"
        report["error"] = safe_error(error)
    report["whole_probe_wall_s"] = time.perf_counter() - started
    report["completed_at"] = now()
    write_json(out / "comparison.json", report)
    print(json.dumps({"status": report["status"], "report": str(out / "comparison.json")}), flush=True)
    return 0 if report["status"] == "passed" else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--source", type=Path, default=Path.cwd())
    parser.add_argument("--worker", choices=("baseline", "candidate"), help=argparse.SUPPRESS)
    parser.add_argument("--case", help=argparse.SUPPRESS)
    args = parser.parse_args()
    if args.worker:
        try:
            return worker(args)
        except BaseException as error:
            write_json(args.out / "worker-preparation-error.json", safe_error(error))
            return 1
    return coordinator(args)


@task
def qualify(ctx, input, out):
    """Start the probe in the exact interpreter selected by normal dda inv."""
    environment = dict(os.environ, JEV_COMPARISON_LAUNCHER="dda inv",
                       JEV_COMPARISON_LAUNCHER_PYTHON=sys.executable)
    result = subprocess.run([sys.executable, "-B", str(Path(__file__).resolve()),
                             "--input", input, "--out", out], env=environment, check=False)
    if result.returncode:
        raise Exit(code=result.returncode)


if __name__ == "__main__":
    sys.exit(main())
