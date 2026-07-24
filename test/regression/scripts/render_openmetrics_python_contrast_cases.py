#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.13"
# dependencies = []
# ///
"""Render focused SMP cases contrasting Python checks with real OpenMetrics.

Run from the repository root:

    test/regression/scripts/render_openmetrics_python_contrast_cases.py
"""

from __future__ import annotations

import shutil
from dataclasses import dataclass
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
CASES = ROOT / "test/regression/cases"
TARGET_COUNTS = (200, 500, 1500)
RUNNERS = 4
DELAY_MS = 100
SCRAPE_INTERVAL_SECONDS = 15
AGGREGATOR_BUFFER_SIZE = 100
CONCURRENT_REQUESTS_MAX = 1000
BUCKETS = ["0.005", "0.01", "0.025", "0.05", "0.1", "0.25", "0.5", "1", "2.5", "5"]
QUANTILES = ["0.5", "0.75", "0.9", "0.95", "0.99"]


@dataclass(frozen=True)
class Shape:
    slug: str
    gauges: int
    counters: int = 0
    histograms: int = 0
    summaries: int = 0

    @property
    def sample_count(self) -> int:
        return (
            2
            + self.counters
            + self.gauges
            + self.histograms * (len(BUCKETS) + 3)
            + self.summaries * (len(QUANTILES) + 2)
        )


@dataclass(frozen=True)
class Scenario:
    slug: str
    check_name: str
    description: str

    def case_name(self, shape: Shape, target_count: int) -> str:
        return f"openmetrics_python_contrast_{self.slug}_{shape.slug}_t{target_count:04d}_r{RUNNERS:02d}"


SHAPES = [
    Shape("samples0962", gauges=960),
]

SCENARIOS = [
    Scenario("noop", "om_noop", "Custom Python check dispatch only; no IO and no per-sample work."),
    Scenario("io_read", "om_io_read", "Custom Python check reads the OpenMetrics-shaped response body."),
    Scenario(
        "parse_submit",
        "om_parse_submit",
        "Custom Python check reads/parses the OpenMetrics-shaped response and submits one gauge per sample.",
    ),
    Scenario("real", "openmetrics", "The real OpenMetrics v2 integration over the same endpoint and payload shape."),
]


def bool_yaml(value: bool) -> str:
    return "true" if value else "false"


def list_yaml(values: list[str]) -> str:
    return "[" + ", ".join(f'"{value}"' for value in values) + "]"


def experiment_yaml(scenario: Scenario, shape: Shape, target_count: int) -> str:
    case_name = scenario.case_name(shape, target_count)
    return f'''# {scenario.description}
optimization_goal: cpu
erratic: false

target:
  name: datadog-agent
  cpu_allotment: 4
  memory_allotment: 4GiB

  environment:
    DD_API_KEY: a0000001
    DD_HOSTNAME: smp-openmetrics-python-contrast
    DD_CHECK_RUNNERS: "{RUNNERS}"

  profiling_environment:
    DD_INTERNAL_PROFILING_BLOCK_PROFILE_RATE: 10000
    DD_INTERNAL_PROFILING_CPU_DURATION: 1m
    DD_INTERNAL_PROFILING_DELTA_PROFILES: true
    DD_INTERNAL_PROFILING_ENABLED: true
    DD_INTERNAL_PROFILING_ENABLE_GOROUTINE_STACKTRACES: true
    DD_INTERNAL_PROFILING_MUTEX_PROFILE_FRACTION: 10
    DD_INTERNAL_PROFILING_PERIOD: 1m
    DD_INTERNAL_PROFILING_UNIX_SOCKET: /smp-host/apm.socket
    DD_PROFILING_EXECUTION_TRACE_ENABLED: true
    DD_PROFILING_EXECUTION_TRACE_PERIOD: 1m
    DD_PROFILING_WAIT_PROFILE: true
    DD_APM_INTERNAL_PROFILING_ENABLED: true
    DD_INTERNAL_PROFILING_EXTRA_TAGS: "experiment:{case_name},workload:openmetrics_python_contrast,scenario:{scenario.slug},target_count:{target_count},check_runners:{RUNNERS},response_delay_ms:{DELAY_MS},payload_samples:{shape.sample_count}"
'''


def datadog_yaml() -> str:
    return f'''auth_token_file_path: /tmp/agent-auth-token
cloud_provider_metadata: []

dd_url: http://127.0.0.1:9091
process_config.process_dd_url: http://localhost:9093

telemetry.enabled: true
telemetry.checks: '*'

log_level: info
aggregator_buffer_size: {AGGREGATOR_BUFFER_SIZE}

dogstatsd_socket: '/tmp/dsd.socket'
'''


def lading_yaml(scenario: Scenario, shape: Shape, target_count: int) -> str:
    return f'''blackhole:
  - http:
      binding_addr: "127.0.0.1:9091"
  - http:
      binding_addr: "127.0.0.1:9093"
  - http:
      binding_addr: "127.0.0.1:9100"
      concurrent_requests_max: {CONCURRENT_REQUESTS_MAX}
      response_delay_millis: {DELAY_MS}
      headers:
        content-type: "text/plain; version=0.0.4"
      body_variant:
        openmetrics:
          metric_name_prefix: "om_contrast"
          include_help: true
          include_type: true
          counters:
            count: {shape.counters}
          gauges:
            count: {shape.gauges}
          histograms:
            count: {shape.histograms}
            buckets: {list_yaml(BUCKETS)}
          summaries:
            count: {shape.summaries}
            quantiles: {list_yaml(QUANTILES)}
          labels:
            services: ["checkout", "catalog", "payments", "fulfillment", "search"]
            regions: ["us-east-1", "us-west-2", "eu-central-1", "ap-southeast-1"]
            methods: ["GET", "POST", "PUT", "DELETE"]
            status_classes: ["2xx", "3xx", "4xx", "5xx"]
            consumers: ["consumer-00", "consumer-01", "consumer-02", "consumer-03", "consumer-04", "consumer-05", "consumer-06", "consumer-07", "consumer-08", "consumer-09", "consumer-10", "consumer-11"]
            route_count: 60

target_metrics:
  - prometheus:
      uri: "http://127.0.0.1:5000/telemetry"
      tags:
        sub_agent: "core"
        workload: "openmetrics_python_contrast"
        scenario: "{scenario.slug}"
        target_count: "{target_count}"
        check_runners: "{RUNNERS}"
        response_delay_ms: "{DELAY_MS}"
        payload_samples: "{shape.sample_count}"
'''


def instance_config(check_name: str, target_count: int) -> str:
    endpoint_key = "openmetrics_endpoint" if check_name == "openmetrics" else "endpoint"
    parts = ["init_config:", "", "instances:"]
    for target in range(1, target_count + 1):
        parts.extend(
            [
                f"  - {endpoint_key}: http://127.0.0.1:9100/metrics?target={target:04d}",
                f"    min_collection_interval: {SCRAPE_INTERVAL_SECONDS}",
            ]
        )
        if check_name == "openmetrics":
            parts.extend(["    metrics: ['.*']", "    max_returned_metrics: 10000"])
    return "\n".join(parts) + "\n"


NOOP_CHECK = '''from datadog_checks.checks import AgentCheck


class OmNoop(AgentCheck):
    def check(self, _instance):
        self.gauge('openmetrics.python_contrast.noop', 1)
'''

IO_READ_CHECK = '''from datadog_checks.checks import AgentCheck


class OmIoRead(AgentCheck):
    def check(self, instance):
        response = self.http.get(instance['endpoint'])
        self.gauge('openmetrics.python_contrast.bytes', len(response.text))
'''

PARSE_SUBMIT_CHECK = '''from datadog_checks.checks import AgentCheck
from prometheus_client.parser import text_string_to_metric_families


class OmParseSubmit(AgentCheck):
    def check(self, instance):
        response = self.http.get(instance['endpoint'])
        submitted = 0
        for family in text_string_to_metric_families(response.text):
            for sample in family.samples:
                self.gauge('openmetrics.python_contrast.sample', sample.value)
                submitted += 1
        self.gauge('openmetrics.python_contrast.submitted', submitted)
'''

CHECK_CONTENT = {
    "om_noop": NOOP_CHECK,
    "om_io_read": IO_READ_CHECK,
    "om_parse_submit": PARSE_SUBMIT_CHECK,
}


def readme() -> str:
    case_lines = "\n".join(
        f"* `{scenario.case_name(shape, target_count)}` — {scenario.slug}, {target_count} targets, {shape.sample_count} OpenMetrics samples"
        for target_count in TARGET_COUNTS
        for shape in SHAPES
        for scenario in SCENARIOS
    )
    return f'''# OpenMetrics Python contrast SMP cases

These cases provide a small, reviewable proof that the OpenMetrics scale problem
is not caused by Python checks being inherently slow.

The suite runs four scenarios at three target counts, using the same 962-sample payload:

1. `noop`: a custom Python check dispatches and submits one gauge.
2. `io_read`: a custom Python check performs HTTP IO and reads an
   OpenMetrics-shaped response body.
3. `parse_submit`: a custom Python check reads/parses the same body and submits
   one gauge per parsed sample.
4. `real`: the real OpenMetrics v2 integration scrapes the same endpoint and
   payload shape.

Cases use `target_count in {200, 500, 1500}`, `DD_CHECK_RUNNERS={RUNNERS}`,
`min_collection_interval={SCRAPE_INTERVAL_SECONDS}s`, and lading response delay
{DELAY_MS}ms. The expected story is that `noop` and `io_read` stay near
scheduler throughput, `parse_submit` shows the controlled cost of Python
parse/submit work, and `real` shows the OpenMetrics-specific degradation.

## Important lading note

The cases use lading's official `body_variant.openmetrics` generator syntax. At
the time this PR was written that generator exists in a custom lading build from
`DataDog/lading#1895` and is being upstreamed. `test/regression/config.yaml`
pins `ghcr.io/datadog/lading:sha-87e2bc917be922a0594bc4a7131cb88516c1e9e6`
so SMP runs use a lading build that includes the generator.

## Cases

{case_lines}

## Running locally

```bash
smp local-run --experiment-dir test/regression \\
  --case openmetrics_python_contrast_real_samples0962_t1500_r04 \\
  --target-image <agent-dev-image>
```

## Regenerating

```bash
test/regression/scripts/render_openmetrics_python_contrast_cases.py
```
'''


def write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)


def render_case(scenario: Scenario, shape: Shape, target_count: int) -> None:
    case_dir = CASES / scenario.case_name(shape, target_count)
    if case_dir.exists():
        shutil.rmtree(case_dir)
    write(case_dir / "experiment.yaml", experiment_yaml(scenario, shape, target_count))
    write(case_dir / "lading/lading.yaml", lading_yaml(scenario, shape, target_count))
    write(case_dir / "datadog-agent/datadog.yaml", datadog_yaml())
    write(
        case_dir / f"datadog-agent/conf.d/{scenario.check_name}.d/conf.yaml",
        instance_config(scenario.check_name, target_count),
    )
    if scenario.check_name in CHECK_CONTENT:
        write(case_dir / f"datadog-agent/checks.d/{scenario.check_name}.py", CHECK_CONTENT[scenario.check_name])


def main() -> None:
    for target_count in TARGET_COUNTS:
        for shape in SHAPES:
            for scenario in SCENARIOS:
                render_case(scenario, shape, target_count)
    write(ROOT / "test/regression/openmetrics_python_contrast.md", readme())
    print("Rendered OpenMetrics Python contrast cases:")
    for target_count in TARGET_COUNTS:
        for shape in SHAPES:
            for scenario in SCENARIOS:
                print(
                    f"  {scenario.case_name(shape, target_count)} targets={target_count} samples={shape.sample_count}"
                )


if __name__ == "__main__":
    main()
