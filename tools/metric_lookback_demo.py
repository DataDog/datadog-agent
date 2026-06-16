#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.11"
# dependencies = ["rich>=13.7.1"]
# ///
"""One-terminal live demo for metric lookback.

Run from the datadog-agent repository root:

    uv run tools/metric_lookback_demo.py

The script builds the agent and fakeintake, starts both in the background, shows
live fakeintake query results, and advances the demo one step at a time when you
press Enter. Press q or Ctrl-C to clean up and exit.
"""

from __future__ import annotations

import argparse
import contextlib
import json
import os
import select
import shutil
import signal
import socket
import subprocess
import sys
import termios
import threading
import time
import tty
from collections import deque
from collections.abc import Callable, Iterable
from dataclasses import dataclass
from pathlib import Path

from rich import box
from rich.console import Console, Group
from rich.live import Live
from rich.panel import Panel
from rich.table import Table
from rich.text import Text

ROOT = Path(__file__).resolve().parents[1]
DEMO_DIR = Path("/tmp/metric-lookback-demo-tui")
FAKEINTAKE_PORT = 18080
CMD_PORT = 15001
DOGSTATSD_PORT = 18125
FAKEINTAKE_URL = f"http://127.0.0.1:{FAKEINTAKE_PORT}"
MANUAL_METRIC = "demo.lookback.shadow"
MANUAL_TAG = "demo:lookback"
TRIGGERED_METRIC = "demo.lookback.triggered"
TRIGGERED_TAG = "demo:trigger"
TRIGGER_METRIC = "demo.lookback.trigger"
TRIGGER_TAG = "demo:lookback"


class Tail:
    def __init__(self, maxlen: int = 200) -> None:
        self._lines: deque[str] = deque(maxlen=maxlen)
        self._lock = threading.Lock()

    def append(self, line: str) -> None:
        line = line.rstrip("\n")
        if not line:
            return
        with self._lock:
            self._lines.append(line)

    def extend_prefixed(self, prefix: str, lines: Iterable[str]) -> None:
        for line in lines:
            self.append(f"{prefix}{line}")

    def last(self, n: int) -> list[str]:
        with self._lock:
            return list(self._lines)[-n:]


@dataclass
class Step:
    title: str
    detail: str
    action: Callable[[], None]


class KeyReader:
    """Non-blocking single-key reader for an interactive terminal."""

    def __init__(self) -> None:
        self.enabled = sys.stdin.isatty()
        self._old: list[int | bytes] | None = None

    def __enter__(self) -> KeyReader:
        if self.enabled:
            self._old = termios.tcgetattr(sys.stdin.fileno())
            tty.setcbreak(sys.stdin.fileno())
        return self

    def __exit__(self, *_: object) -> None:
        if self.enabled and self._old is not None:
            termios.tcsetattr(sys.stdin.fileno(), termios.TCSADRAIN, self._old)

    def read_key(self) -> str | None:
        if not self.enabled:
            return None
        readable, _, _ = select.select([sys.stdin], [], [], 0)
        if not readable:
            return None
        return sys.stdin.read(1)


class Demo:
    def __init__(self, *, skip_build: bool, no_cleanup: bool) -> None:
        self.console = Console()
        self.skip_build = skip_build
        self.no_cleanup = no_cleanup
        self.agent_bin = ROOT / "bin/agent/agent"
        self.fakeintake_bin = ROOT / "test/fakeintake/build/fakeintake"
        self.fakeintakectl_bin = ROOT / "test/fakeintake/build/fakeintakectl"
        self.config_path = DEMO_DIR / "datadog.yaml"

        self.command_log = Tail(260)
        self.agent_log = Tail(160)
        self.fakeintake_log = Tail(80)

        self.fakeintake_proc: subprocess.Popen[str] | None = None
        self.agent_proc: subprocess.Popen[str] | None = None
        self.current_cmd_proc: subprocess.Popen[str] | None = None

        self.fakeintake_ready = False
        self.agent_ready = False
        self.built = False
        self.action_running = False
        self.action_error: str | None = None
        self.last_action = "Ready. Press Enter to build demo binaries."
        self.current_step = 0
        self.done_steps: set[int] = set()
        self.stop_event = threading.Event()
        self.poll_thread: threading.Thread | None = None
        self.action_thread: threading.Thread | None = None
        self.lock = threading.Lock()

        self.route_stats = "fakeintake not started"
        self.manual_series: list[dict] = []
        self.triggered_series: list[dict] = []
        self.trigger_signal_series: list[dict] = []

        self.steps: list[Step] = [
            Step(
                "Build binaries",
                "Run dda inv agent.build and dda inv fakeintake.build.",
                self.step_build,
            ),
            Step(
                "Start services",
                "Write demo config, start fakeintake + agent, then flush fakeintake.",
                self.step_start_services,
            ),
            Step(
                "Normal flow sanity check",
                "Send a DogStatsD metric, then dump: lookback should still report 0.",
                self.step_normal_flow_check,
            ),
            Step(
                "Seed lookback buffer",
                "Use the hidden seed command; fakeintake should still not show the metric.",
                self.step_seed_manual,
            ),
            Step(
                "Manual dump",
                "Run metric-lookback-dump; fakeintake should show demo.lookback.shadow=42.",
                self.step_manual_dump,
            ),
            Step(
                "Prepare trigger demo",
                "Flush fakeintake and seed a new lookback sample. Capacity=1 keeps this view clean.",
                self.step_prepare_trigger,
            ),
            Step(
                "DogStatsD trigger",
                "Send demo.lookback.trigger:1|g; trigger session dumps after the configured 17s delay.",
                self.step_send_trigger,
            ),
            Step(
                "Stop services",
                "Terminate the demo agent and fakeintake.",
                self.step_stop_services,
            ),
        ]

    # ----- process helpers -------------------------------------------------

    def run_cmd(self, args: list[str], *, timeout: float | None = None, ok_rcs: set[int] | None = None) -> str:
        ok_rcs = ok_rcs or {0}
        self.command_log.append("$ " + " ".join(args))
        proc = subprocess.Popen(
            args,
            cwd=ROOT,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,
        )
        self.current_cmd_proc = proc
        output: list[str] = []
        start = time.monotonic()
        try:
            assert proc.stdout is not None
            while True:
                if timeout is not None and time.monotonic() - start > timeout:
                    proc.terminate()
                    raise TimeoutError(f"timed out after {timeout:.0f}s: {' '.join(args)}")
                line = proc.stdout.readline()
                if line:
                    output.append(line)
                    self.command_log.append(line)
                    continue
                if proc.poll() is not None:
                    rest = proc.stdout.read()
                    if rest:
                        output.append(rest)
                        self.command_log.extend_prefixed("", rest.splitlines())
                    break
                time.sleep(0.05)
            rc = proc.wait()
        finally:
            if self.current_cmd_proc is proc:
                self.current_cmd_proc = None
        if rc not in ok_rcs:
            raise RuntimeError(f"command failed ({rc}): {' '.join(args)}")
        return "".join(output)

    def start_process(self, name: str, args: list[str], tail: Tail) -> subprocess.Popen[str]:
        tail.append("$ " + " ".join(args))
        proc = subprocess.Popen(
            args,
            cwd=ROOT,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,
            start_new_session=True,
        )

        def reader() -> None:
            assert proc.stdout is not None
            for line in proc.stdout:
                tail.append(line)

        threading.Thread(target=reader, name=f"{name}-log-reader", daemon=True).start()
        return proc

    def terminate_process(self, proc: subprocess.Popen[str] | None, name: str) -> None:
        if proc is None or proc.poll() is not None:
            return
        self.command_log.append(f"stopping {name} (pid {proc.pid})")
        with contextlib.suppress(ProcessLookupError):
            os.killpg(proc.pid, signal.SIGTERM)
        try:
            proc.wait(timeout=8)
        except subprocess.TimeoutExpired:
            self.command_log.append(f"{name} did not stop after SIGTERM; killing")
            with contextlib.suppress(ProcessLookupError):
                os.killpg(proc.pid, signal.SIGKILL)
            proc.wait(timeout=5)

    @staticmethod
    def port_open(port: int, host: str = "127.0.0.1") -> bool:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
            sock.settimeout(0.2)
            return sock.connect_ex((host, port)) == 0

    def wait_for_port(self, port: int, label: str, timeout: float = 60) -> None:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if self.port_open(port):
                return
            time.sleep(0.25)
        raise TimeoutError(f"{label} did not listen on port {port} within {timeout:.0f}s")

    def check_ports_free(self) -> None:
        in_use = [p for p in (FAKEINTAKE_PORT, CMD_PORT, DOGSTATSD_PORT) if self.port_open(p)]
        if in_use:
            raise RuntimeError(f"ports already in use: {in_use}")

    # ----- fakeintake/agent helpers ---------------------------------------

    def fakeintakectl(self, *args: str, ok_rcs: set[int] | None = None, timeout: float = 20) -> str:
        return self.run_cmd(
            [str(self.fakeintakectl_bin), "--url", FAKEINTAKE_URL, *args], timeout=timeout, ok_rcs=ok_rcs
        )

    def agent_cli(self, *args: str, timeout: float = 60) -> str:
        return self.run_cmd([str(self.agent_bin), *args, "-c", str(self.config_path)], timeout=timeout)

    def send_dogstatsd(self, line: str) -> None:
        self.command_log.append(f"UDP 127.0.0.1:{DOGSTATSD_PORT} {line.strip()}")
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
            sock.sendto(line.encode(), ("127.0.0.1", DOGSTATSD_PORT))

    def write_config(self) -> None:
        if DEMO_DIR.exists():
            shutil.rmtree(DEMO_DIR)
        for subdir in ("run", "logs", "conf.d", "checks.d"):
            (DEMO_DIR / subdir).mkdir(parents=True, exist_ok=True)
        self.config_path.write_text(
            f"""api_key: "00000000000000000000000000000000"
hostname: lookback-demo
dd_url: {FAKEINTAKE_URL}

use_v2_api:
  series: true

cmd_port: {CMD_PORT}
dogstatsd_port: {DOGSTATSD_PORT}

run_path: {DEMO_DIR}/run
log_file: {DEMO_DIR}/logs/agent.log
log_to_console: true
log_level: info

confd_path: {DEMO_DIR}/conf.d
additional_checksd: {DEMO_DIR}/checks.d

remote_configuration:
  enabled: false

metric_lookback:
  enabled: true
  capacity: 1
  shard_count: 1
  debug_seed:
    enabled: true
  trigger:
    enabled: true
    metric_name: {TRIGGER_METRIC}
    threshold: 1
    ewma_alpha: 1
    cooldown: 30s
    pre_window: 15s
    post_window: 15s
    dump_interval: 10s
    send_delay: 17s
""",
            encoding="utf-8",
        )
        self.command_log.append(f"wrote {self.config_path}")

    def wait_until_metric(self, name: str, tag: str, timeout: float = 20) -> None:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            metrics = self.query_metric(name, tag)
            if metrics:
                return
            time.sleep(0.5)
        raise TimeoutError(f"fakeintake did not receive {name} with tag {tag}")

    # ----- step actions ----------------------------------------------------

    def step_build(self) -> None:
        if self.skip_build:
            self.command_log.append("--skip-build set; verifying existing artifacts")
        else:
            self.run_cmd(["dda", "inv", "agent.build", "--build-exclude=systemd"], timeout=1800)
            self.run_cmd(["dda", "inv", "fakeintake.build"], timeout=1200)
        missing = [str(p) for p in (self.agent_bin, self.fakeintake_bin, self.fakeintakectl_bin) if not p.exists()]
        if missing:
            raise RuntimeError("missing build artifacts: " + ", ".join(missing))
        self.built = True
        self.last_action = "Build artifacts are ready."

    def step_start_services(self) -> None:
        self.check_ports_free()
        self.write_config()
        self.fakeintake_proc = self.start_process(
            "fakeintake",
            [str(self.fakeintake_bin), "-port", str(FAKEINTAKE_PORT), "-retention-period", "30m"],
            self.fakeintake_log,
        )
        self.wait_for_port(FAKEINTAKE_PORT, "fakeintake")
        self.fakeintake_ready = True
        self.agent_proc = self.start_process(
            "agent", [str(self.agent_bin), "run", "-c", str(self.config_path)], self.agent_log
        )
        self.wait_for_port(CMD_PORT, "agent command API", timeout=90)
        self.agent_ready = True
        # Give startup payloads a moment, then make the visual baseline clean.
        time.sleep(1)
        self.fakeintakectl("flush")
        self.last_action = "fakeintake and agent are running; fakeintake has been flushed."

    def step_normal_flow_check(self) -> None:
        self.fakeintakectl("flush")
        self.send_dogstatsd(f"demo.normal:7|g|#{MANUAL_TAG}\n")
        # DogStatsD normal flow should not write the lookback buffer, so the
        # manual dump should report zero series even though normal metrics may
        # be flushed later through the ordinary aggregator path.
        out = self.agent_cli("metric-lookback-dump")
        if "Dumped 0" not in out:
            raise RuntimeError("expected lookback dump to report zero series after normal DogStatsD sample")
        self.last_action = "Normal DogStatsD metric did not populate the lookback buffer (dumped 0)."

    def step_seed_manual(self) -> None:
        self.agent_cli("metric-lookback-seed", "--metric", MANUAL_METRIC, "--value", "42", "--tag", MANUAL_TAG)
        time.sleep(0.5)
        if self.query_metric(MANUAL_METRIC, MANUAL_TAG):
            raise RuntimeError("manual metric reached fakeintake before dump")
        self.last_action = "Seeded ring buffer via lookback sender; fakeintake still does not have the metric."

    def step_manual_dump(self) -> None:
        out = self.agent_cli("metric-lookback-dump")
        if "Dumped 1" not in out:
            raise RuntimeError("expected manual dump to report one series")
        self.wait_until_metric(MANUAL_METRIC, MANUAL_TAG)
        self.last_action = "Manual dump sent demo.lookback.shadow=42 through serializer/forwarder to fakeintake."

    def step_prepare_trigger(self) -> None:
        self.fakeintakectl("flush")
        self.agent_cli("metric-lookback-seed", "--metric", TRIGGERED_METRIC, "--value", "99", "--tag", TRIGGERED_TAG)
        time.sleep(0.5)
        if self.query_metric(TRIGGERED_METRIC, TRIGGERED_TAG):
            raise RuntimeError("trigger demo metric reached fakeintake before trigger")
        self.last_action = "Trigger demo sample is retained in the ring buffer; fakeintake is still clean."

    def step_send_trigger(self) -> None:
        self.send_dogstatsd(f"{TRIGGER_METRIC}:1|g|#{TRIGGER_TAG}\n")
        self.wait_until_metric(TRIGGERED_METRIC, TRIGGERED_TAG, timeout=35)
        self.last_action = "DogStatsD trigger fired; delayed dump session sent the retained trigger demo sample."

    def step_stop_services(self) -> None:
        self.cleanup_processes()
        self.last_action = "Demo services stopped. Press q to exit, or Enter to do nothing."

    # ----- polling ---------------------------------------------------------

    def query_metric(self, name: str, tag: str) -> list[dict]:
        if not self.fakeintake_ready:
            return []
        try:
            proc = subprocess.run(
                [
                    str(self.fakeintakectl_bin),
                    "--url",
                    FAKEINTAKE_URL,
                    "filter",
                    "metrics",
                    "--name",
                    name,
                    "--tags",
                    tag,
                ],
                cwd=ROOT,
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                timeout=5,
                check=False,
            )
        except Exception:
            return []
        raw = proc.stdout.strip()
        if not raw:
            return []
        try:
            parsed = json.loads(raw)
        except json.JSONDecodeError:
            return []
        return parsed if isinstance(parsed, list) else []

    def poll_loop(self) -> None:
        while not self.stop_event.is_set():
            if self.fakeintake_ready:
                try:
                    proc = subprocess.run(
                        [str(self.fakeintakectl_bin), "--url", FAKEINTAKE_URL, "route-stats"],
                        cwd=ROOT,
                        stdin=subprocess.DEVNULL,
                        capture_output=True,
                        text=True,
                        timeout=5,
                        check=False,
                    )
                    if proc.stdout.strip():
                        self.route_stats = proc.stdout.strip()
                    self.manual_series = self.query_metric(MANUAL_METRIC, MANUAL_TAG)
                    self.triggered_series = self.query_metric(TRIGGERED_METRIC, TRIGGERED_TAG)
                    self.trigger_signal_series = self.query_metric(TRIGGER_METRIC, TRIGGER_TAG)
                except Exception as exc:  # noqa: BLE001 - diagnostics only
                    self.route_stats = f"poll error: {exc}"
            self.fakeintake_ready = (
                self.fakeintake_proc is not None
                and self.fakeintake_proc.poll() is None
                and self.port_open(FAKEINTAKE_PORT)
            )
            self.agent_ready = (
                self.agent_proc is not None and self.agent_proc.poll() is None and self.port_open(CMD_PORT)
            )
            time.sleep(1)

    # ----- UI --------------------------------------------------------------

    def start_next_step(self) -> None:
        if self.action_running:
            return
        if self.current_step >= len(self.steps):
            self.last_action = "Demo complete. Press q to exit."
            return

        idx = self.current_step
        step = self.steps[idx]

        def runner() -> None:
            with self.lock:
                self.action_running = True
                self.action_error = None
                self.last_action = f"Running: {step.title}"
            try:
                step.action()
            except Exception as exc:  # noqa: BLE001 - surfaced in TUI
                with self.lock:
                    self.action_error = str(exc)
                    self.last_action = f"FAILED: {step.title}"
                self.command_log.append(f"ERROR: {exc}")
            else:
                with self.lock:
                    self.done_steps.add(idx)
                    self.current_step += 1
            finally:
                with self.lock:
                    self.action_running = False

        self.action_thread = threading.Thread(target=runner, name="metric-lookback-demo-action", daemon=True)
        self.action_thread.start()

    def render(self) -> Group:
        return Group(
            self.render_header(),
            self.render_steps(),
            self.render_status(),
            self.render_metrics(),
            self.render_logs(),
        )

    def render_header(self) -> Panel:
        if self.action_running:
            control = "Working…  q: quit/cleanup"
            style = "bold yellow"
        elif self.current_step < len(self.steps):
            control = f"Enter: {self.steps[self.current_step].title}    q: quit/cleanup"
            style = "bold cyan"
        else:
            control = "Demo complete. q: quit/cleanup"
            style = "bold green"
        text = Text()
        text.append("Metric Lookback Live Demo\n", style="bold white")
        text.append(control, style=style)
        if self.action_error:
            text.append(f"\nLast error: {self.action_error}", style="bold red")
        else:
            text.append(f"\n{self.last_action}", style="green" if not self.action_running else "yellow")
        return Panel(text, box=box.ROUNDED)

    def render_steps(self) -> Panel:
        table = Table.grid(expand=True)
        table.add_column(ratio=1)
        table.add_column(ratio=5)
        for i, step in enumerate(self.steps):
            if i in self.done_steps:
                marker = "✓"
                style = "green"
            elif i == self.current_step:
                marker = "▶" if self.action_running else "•"
                style = "bold yellow"
            else:
                marker = " "
                style = "dim"
            table.add_row(Text(marker, style=style), Text(f"{step.title}: {step.detail}", style=style))
        return Panel(table, title="Demo steps", box=box.ROUNDED)

    def render_status(self) -> Panel:
        table = Table(box=box.SIMPLE, expand=True)
        table.add_column("Thing")
        table.add_column("State")
        table.add_row("agent binary", self.exists_text(self.agent_bin))
        table.add_row("fakeintake binary", self.exists_text(self.fakeintake_bin))
        table.add_row("fakeintakectl", self.exists_text(self.fakeintakectl_bin))
        table.add_row("fakeintake", self.ready_text(self.fakeintake_ready, FAKEINTAKE_PORT))
        table.add_row("agent IPC", self.ready_text(self.agent_ready, CMD_PORT))
        table.add_row("DogStatsD", f"UDP 127.0.0.1:{DOGSTATSD_PORT}")
        table.add_row("config", str(self.config_path))
        table.add_row("fakeintake routes", self.route_stats)
        return Panel(table, title="Live services", box=box.ROUNDED)

    @staticmethod
    def exists_text(path: Path) -> Text:
        return Text("✓ " + str(path), style="green") if path.exists() else Text("missing", style="red")

    @staticmethod
    def ready_text(ready: bool, port: int) -> Text:
        return Text(f"✓ listening on {port}", style="green") if ready else Text(f"not listening on {port}", style="red")

    def render_metrics(self) -> Panel:
        table = Table(box=box.SIMPLE_HEAVY, expand=True)
        table.add_column("Metric")
        table.add_column("Expected story")
        table.add_column("Fakeintake result")
        table.add_row(
            MANUAL_METRIC,
            "absent after seed; present after manual dump",
            self.metric_text(self.manual_series, "42"),
        )
        table.add_row(
            TRIGGERED_METRIC,
            "absent after seed; present after DogStatsD trigger",
            self.metric_text(self.triggered_series, "99"),
        )
        table.add_row(
            TRIGGER_METRIC,
            "ordinary DogStatsD signal; may appear after normal flush",
            self.metric_text(self.trigger_signal_series, "1"),
        )
        return Panel(table, title="Fakeintake metric queries", box=box.ROUNDED)

    @staticmethod
    def metric_text(series: list[dict], expected_value: str) -> Text:
        if not series:
            return Text("❌ not received", style="red")
        parts: list[str] = []
        for serie in series[:3]:
            points = serie.get("points") or []
            values = ",".join(str(point.get("value")) for point in points[:3]) or "no-points"
            tags = ",".join(serie.get("tags") or [])
            parts.append(f"✅ values=[{values}] tags=[{tags}]")
        style = "green" if expected_value in " ".join(parts) else "yellow"
        return Text("\n".join(parts), style=style)

    def render_logs(self) -> Panel:
        lines: list[str] = []
        lines.append("command output:")
        lines.extend("  " + line for line in self.command_log.last(10))
        lookback_agent = [line for line in self.agent_log.last(60) if "lookback" in line.lower()]
        if lookback_agent:
            lines.append("")
            lines.append("agent lookback log lines:")
            lines.extend("  " + line for line in lookback_agent[-6:])
        fakeintake_lines = self.fakeintake_log.last(3)
        if fakeintake_lines:
            lines.append("")
            lines.append("fakeintake tail:")
            lines.extend("  " + line for line in fakeintake_lines)
        return Panel("\n".join(lines[-24:]) or "no logs yet", title="Recent logs", box=box.ROUNDED)

    # ----- lifecycle -------------------------------------------------------

    def cleanup_processes(self) -> None:
        self.terminate_process(self.current_cmd_proc, "foreground command")
        self.terminate_process(self.agent_proc, "agent")
        self.terminate_process(self.fakeintake_proc, "fakeintake")
        self.agent_proc = None
        self.fakeintake_proc = None
        self.agent_ready = False
        self.fakeintake_ready = False

    def run(self) -> None:
        self.poll_thread = threading.Thread(target=self.poll_loop, name="metric-lookback-demo-poller", daemon=True)
        self.poll_thread.start()
        with KeyReader() as keys, Live(self.render(), console=self.console, refresh_per_second=4, screen=True) as live:
            try:
                while not self.stop_event.is_set():
                    key = keys.read_key()
                    if key in {"q", "Q"}:
                        break
                    if key in {"\r", "\n"}:
                        self.start_next_step()
                    live.update(self.render())
                    time.sleep(0.25)
            except KeyboardInterrupt:
                pass
            finally:
                self.stop_event.set()
                if not self.no_cleanup:
                    self.cleanup_processes()
                live.update(self.render())


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run the one-terminal metric lookback live demo.")
    parser.add_argument(
        "--skip-build", action="store_true", help="Use existing bin/agent/agent and fakeintake binaries."
    )
    parser.add_argument(
        "--no-cleanup", action="store_true", help="Leave background agent/fakeintake processes running on exit."
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if not (ROOT / "go.mod").exists():
        print("Run this script from inside the datadog-agent checkout.", file=sys.stderr)
        return 2
    demo = Demo(skip_build=args.skip_build, no_cleanup=args.no_cleanup)
    demo.run()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
