#!/usr/bin/env python3

"""Run a local adaptive-sampler repro with two file sources for stream-logs.

This script starts a local Agent configured with two tailed file sources:

1. `sampler_enabled` with per-source adaptive sampling enabled
2. `sampler_disabled` with per-source adaptive sampling disabled

Both files are continuously written to so the behavior can be inspected with
`agent stream-logs`.
"""

from __future__ import annotations

import argparse
import atexit
import pathlib
import shutil
import signal
import socket
import subprocess
import tempfile
import textwrap
import threading
import time
import urllib.error
import urllib.request
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


ENABLED_SERVICE = "adaptive-sampler-enabled"
DISABLED_SERVICE = "adaptive-sampler-disabled"
ENABLED_SOURCE = "sampler_enabled"
DISABLED_SOURCE = "sampler_disabled"


class HarnessError(RuntimeError):
    """Raised when the harness cannot complete successfully."""


class ManagedProcess:
    """Tracks a subprocess and its log file."""

    def __init__(self, name: str, popen: subprocess.Popen[str], log_path: pathlib.Path) -> None:
        self.name = name
        self.popen = popen
        self.log_path = log_path

    def terminate(self) -> None:
        if self.popen.poll() is not None:
            return
        self.popen.terminate()
        try:
            self.popen.wait(timeout=10)
        except subprocess.TimeoutExpired:
            self.popen.kill()
            self.popen.wait(timeout=5)

    def tail(self, lines: int = 80) -> str:
        if not self.log_path.exists():
            return f"<missing log file: {self.log_path}>"
        content = self.log_path.read_text(encoding="utf-8", errors="replace").splitlines()
        return "\n".join(content[-lines:])


class DummyIntakeHandler(BaseHTTPRequestHandler):
    """Accepts any POST and returns 200 so the Agent can drain its pipelines."""

    def do_GET(self) -> None:  # noqa: N802
        if self.path == "/health":
            self.send_response(HTTPStatus.OK)
            self.end_headers()
            self.wfile.write(b"ok\n")
            return
        self.send_response(HTTPStatus.NOT_FOUND)
        self.end_headers()

    def do_POST(self) -> None:  # noqa: N802
        length = int(self.headers.get("Content-Length", "0"))
        if length:
            _ = self.rfile.read(length)
        self.send_response(HTTPStatus.OK)
        self.end_headers()
        self.wfile.write(b"{}\n")

    def log_message(self, format: str, *args: object) -> None:  # noqa: A003
        return


class RepeatingWriter(threading.Thread):
    """Continuously appends logs to a file until told to stop."""

    def __init__(self, path: pathlib.Path, template: str, interval: float, stop_event: threading.Event) -> None:
        super().__init__(daemon=True)
        self.path = path
        self.template = template
        self.interval = interval
        self.stop_event = stop_event

    def run(self) -> None:
        count = 0
        with self.path.open("a", encoding="utf-8", buffering=1) as handle:
            while not self.stop_event.is_set():
                count += 1
                handle.write(self.template.format(count=count) + "\n")
                handle.flush()
                time.sleep(self.interval)


class AdaptiveSamplerStreamlogsHarness:
    def __init__(self, args: argparse.Namespace) -> None:
        self.args = args
        self.repo_root = pathlib.Path(__file__).resolve().parents[1]
        self.agent_binary = self._resolve_agent_binary(args.agent_binary)
        self.work_dir = pathlib.Path(
            tempfile.mkdtemp(prefix="adaptive-sampler-streamlogs-", dir=args.temp_root)
        )
        self.confd_dir = self.work_dir / "conf.d" / "adaptive_sampler_repro.d"
        self.run_dir = self.work_dir / "run"
        self.enabled_log_path = self.work_dir / "sampling-enabled.log"
        self.disabled_log_path = self.work_dir / "sampling-disabled.log"
        self.datadog_yaml_path = self.work_dir / "datadog.yaml"
        self.integration_yaml_path = self.confd_dir / "conf.yaml"
        self.agent_log_path = self.work_dir / "agent.log"
        self.cmd_port = args.cmd_port or self._pick_free_port()
        self.expvar_port = args.expvar_port or self._pick_free_port()
        self.intake_port = args.intake_port or self._pick_free_port()
        self.stop_event = threading.Event()
        self.server: ThreadingHTTPServer | None = None
        self.server_thread: threading.Thread | None = None
        self.agent: ManagedProcess | None = None
        self.writers: list[RepeatingWriter] = []

    def run(self) -> int:
        try:
            self._prepare_files()
            self._start_dummy_intake()
            self._start_agent()
            self._wait_for_http(
                f"http://127.0.0.1:{self.expvar_port}/telemetry",
                "agent telemetry",
                timeout=90,
            )
            self._start_writers()
            self._print_instructions()
            self._wait()
            return 0
        except KeyboardInterrupt:
            return 0
        except Exception as err:  # noqa: BLE001
            self._print_failure(err)
            return 1
        finally:
            self.cleanup()

    def cleanup(self) -> None:
        self.stop_event.set()
        for writer in self.writers:
            writer.join(timeout=2)

        if self.agent is not None:
            self.agent.terminate()

        if self.server is not None:
            self.server.shutdown()
            self.server.server_close()
        if self.server_thread is not None:
            self.server_thread.join(timeout=2)

        if not self.args.keep_temp:
            shutil.rmtree(self.work_dir, ignore_errors=True)

    def _prepare_files(self) -> None:
        self.confd_dir.mkdir(parents=True, exist_ok=True)
        self.run_dir.mkdir(parents=True, exist_ok=True)
        self.enabled_log_path.touch()
        self.disabled_log_path.touch()

        datadog_yaml = textwrap.dedent(
            f"""\
            api_key: "00000000000000000000000000000000"
            site: datadoghq.com
            hostname: adaptive-sampler-streamlogs
            logs_enabled: true
            log_level: info
            log_file: "{self.agent_log_path}"
            confd_path: "{self.work_dir / 'conf.d'}"
            run_path: "{self.run_dir}"
            cmd_port: {self.cmd_port}
            expvar_port: {self.expvar_port}
            dogstatsd_port: 0
            logs_config:
              run_path: "{self.run_dir}"
              logs_dd_url: "127.0.0.1:{self.intake_port}"
              logs_no_ssl: true
              force_use_http: true
              experimental_adaptive_sampling:
                enabled: false
                max_patterns: 100
                rate_limit: 1
                burst_size: 1
                match_threshold: 0.9
                tokenizer_max_input_bytes: 2048
            """
        )
        self.datadog_yaml_path.write_text(datadog_yaml, encoding="utf-8")

        integration_yaml = textwrap.dedent(
            f"""\
            logs:
              - type: file
                path: "{self.enabled_log_path}"
                service: "{ENABLED_SERVICE}"
                source: "{ENABLED_SOURCE}"
                start_position: beginning
                experimental_adaptive_sampling:
                  enabled: true

              - type: file
                path: "{self.disabled_log_path}"
                service: "{DISABLED_SERVICE}"
                source: "{DISABLED_SOURCE}"
                start_position: beginning
                experimental_adaptive_sampling:
                  enabled: false
            """
        )
        self.integration_yaml_path.write_text(integration_yaml, encoding="utf-8")

    def _start_dummy_intake(self) -> None:
        self.server = ThreadingHTTPServer(("127.0.0.1", self.intake_port), DummyIntakeHandler)
        self.server_thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.server_thread.start()
        self._wait_for_http(f"http://127.0.0.1:{self.intake_port}/health", "dummy intake", timeout=30)

    def _start_agent(self) -> None:
        log_handle = self.agent_log_path.open("w", encoding="utf-8")
        popen = subprocess.Popen(
            [
                str(self.agent_binary),
                "run",
                "-c",
                str(self.datadog_yaml_path),
            ],
            cwd=self.repo_root,
            stdout=log_handle,
            stderr=subprocess.STDOUT,
            text=True,
        )
        self.agent = ManagedProcess("agent", popen, self.agent_log_path)

    def _start_writers(self) -> None:
        enabled_writer = RepeatingWriter(
            self.enabled_log_path,
            "this file has sampling enabled count={count}",
            self.args.write_interval,
            self.stop_event,
        )
        disabled_writer = RepeatingWriter(
            self.disabled_log_path,
            "this one has sampling disabled count={count}",
            self.args.write_interval,
            self.stop_event,
        )
        self.writers = [enabled_writer, disabled_writer]
        for writer in self.writers:
            writer.start()

    def _wait(self) -> None:
        if self.args.run_for > 0:
            time.sleep(self.args.run_for)
            return
        while not self.stop_event.is_set():
            time.sleep(0.5)

    def _print_instructions(self) -> None:
        enabled_cmd = (
            f"{self.agent_binary} stream-logs -c {self.datadog_yaml_path} "
            f"--source {ENABLED_SOURCE} --duration 10s"
        )
        disabled_cmd = (
            f"{self.agent_binary} stream-logs -c {self.datadog_yaml_path} "
            f"--source {DISABLED_SOURCE} --duration 10s"
        )

        print("")
        print("Adaptive sampler stream-logs repro is running.")
        print(f"Work dir: {self.work_dir}")
        print(f"Agent log: {self.agent_log_path}")
        print(f"Enabled file: {self.enabled_log_path}")
        print(f"Disabled file: {self.disabled_log_path}")
        print("")
        print("Try these commands in another terminal:")
        print(f"  {enabled_cmd}")
        print(f"  {disabled_cmd}")
        print("")
        print("Expected behavior:")
        print(f"  - {ENABLED_SOURCE}: fewer emitted logs and occasional adaptive_sampler_sampled_count tags")
        print(f"  - {DISABLED_SOURCE}: continuous unsampled logs with no adaptive sampler tag")
        print("")
        if self.args.run_for > 0:
            print(f"The harness will stop automatically after {self.args.run_for:.1f}s.")
        else:
            print("Press Ctrl-C to stop.")

    def _print_failure(self, err: Exception) -> None:
        print("")
        print(f"FAIL: {err}")
        if self.agent is not None:
            print("")
            print("--- Agent log tail ---")
            print(self.agent.tail())
        print(f"Work dir: {self.work_dir}")

    def _resolve_agent_binary(self, agent_binary: str | None) -> pathlib.Path:
        binary = pathlib.Path(agent_binary) if agent_binary else self.repo_root / "bin" / "agent" / "agent"
        if not binary.exists():
            raise HarnessError(f"agent binary not found at {binary}")
        return binary.resolve()

    def _pick_free_port(self) -> int:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
            sock.bind(("127.0.0.1", 0))
            return int(sock.getsockname()[1])

    def _wait_for_http(self, url: str, name: str, timeout: float) -> None:
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                with urllib.request.urlopen(url, timeout=2) as response:
                    if response.status == 200:
                        return
            except (OSError, urllib.error.URLError):
                time.sleep(0.25)
        raise HarnessError(f"timed out waiting for {name} at {url}")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--agent-binary", help="Path to the built agent binary")
    parser.add_argument("--temp-root", help="Directory under which to create the temp workspace")
    parser.add_argument("--keep-temp", action="store_true", help="Keep the temp workspace after exit")
    parser.add_argument("--run-for", type=float, default=0.0, help="Stop automatically after N seconds")
    parser.add_argument(
        "--write-interval",
        type=float,
        default=0.05,
        help="Seconds between log writes for each file source",
    )
    parser.add_argument("--cmd-port", type=int, help="Agent cmd_port override")
    parser.add_argument("--expvar-port", type=int, help="Agent expvar_port override")
    parser.add_argument("--intake-port", type=int, help="Dummy intake port override")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    harness = AdaptiveSamplerStreamlogsHarness(args)
    atexit.register(harness.cleanup)
    signal.signal(signal.SIGTERM, lambda *_: harness.stop_event.set())
    return harness.run()


if __name__ == "__main__":
    raise SystemExit(main())
