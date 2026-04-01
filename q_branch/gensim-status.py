#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "textual>=0.50",
# ]
# ///
"""
Gensim EKS Episode Evaluation Monitor

Real-time TUI that tracks a gensim-eks evaluation run by polling:
  - Pulumi state file for infra resource counts
  - kubectl configmap for episode plan/status
  - kubectl logs for orchestrator output
  - AWS EKS cluster status (optional)

Usage:
    # Remote (EKS) — monitor an evaluation run on gensim-eks
    aws-vault exec sso-agent-sandbox-account-admin -- uv run q_branch/gensim-status.py

    # Local (Kind) — monitor a local episode run
    uv run q_branch/gensim-status.py --local

    # Other useful commands
    dda inv aws.eks.gensim.status    # Quick non-interactive status check (EKS)
    dda inv q.run-local-episode ...  # Start a local episode run
"""

from __future__ import annotations

import asyncio
import json
import os
import re
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from textual.app import App, ComposeResult
from textual.binding import Binding
from textual.containers import Horizontal
from textual.reactive import reactive
from textual.widgets import Footer, Header, Static

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------

# ---------------------------------------------------------------------------
# Mode detection: --local flag switches to Kind cluster, skips Pulumi/EKS
# ---------------------------------------------------------------------------
LOCAL_MODE = "--local" in sys.argv

# Auto-detect stack name from the user prefix (matches e2e framework convention).
_USER = os.environ.get("USER", "unknown").replace(".", "-")
_STACK_NAME = os.environ.get("GENSIM_STACK_NAME", f"{_USER}-gensim-eks")

if LOCAL_MODE:
    _LOCAL_CLUSTER = os.environ.get("LOCAL_CLUSTER_NAME", "observer-local")
    KUBECONFIG = os.path.expanduser("~/.kube/config")
    PULUMI_STATE = "/dev/null"  # no Pulumi for local
    EKS_CLUSTER_NAME = ""
    LOCAL_EPISODE_LOG = "/tmp/local-episode-runner.log"
else:
    _LOCAL_CLUSTER = ""
    LOCAL_EPISODE_LOG = ""
    # All paths derived from stack name -- override via env vars if needed.
    KUBECONFIG = os.environ.get("KUBECONFIG", f"{_STACK_NAME}-kubeconfig.yaml")
    PULUMI_STATE = os.path.expanduser(os.environ.get("PULUMI_STATE", f"~/.pulumi/stacks/{_STACK_NAME}.json"))
    EKS_CLUSTER_NAME = os.environ.get("EKS_CLUSTER_NAME", _STACK_NAME)

EKS_REGION = os.environ.get("EKS_REGION", "us-east-1")

KUBECTL_ENV = {**os.environ, "KUBECONFIG": KUBECONFIG}
if LOCAL_MODE:
    # For Kind, we need the context in all kubectl calls. The easiest way is to
    # set KUBECTL_CONTEXT which some helpers use, and inject --context into
    # fetch_configmap. For the generic kubectl calls (pods, nodes) that go via
    # run_cmd, we just set the current-context in KUBECTL_ENV.
    KUBECTL_ENV["KUBECTL_CONTEXT"] = f"kind-{_LOCAL_CLUSTER}"

# Phase durations (seconds) extracted from typical gensim orchestrator defaults.
PHASE_DURATIONS: dict[str, int] = {
    "warmup": 120,
    "baseline": 600,
    "disruption": 300,
    "observation": 300,
    "cooldown": 60,
}

# ---------------------------------------------------------------------------
# Async subprocess helper
# ---------------------------------------------------------------------------


async def run_cmd(cmd: list[str], env: dict | None = None, timeout: float = 15) -> str:
    """Run a command asynchronously, returning stdout or empty string on failure."""
    try:
        proc = await asyncio.create_subprocess_exec(
            *cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            env=env,
        )
        stdout, _ = await asyncio.wait_for(proc.communicate(), timeout=timeout)
        if proc.returncode == 0:
            return stdout.decode("utf-8", errors="replace").strip()
        return ""
    except (TimeoutError, FileNotFoundError, OSError):
        return ""


# ---------------------------------------------------------------------------
# Data fetchers
# ---------------------------------------------------------------------------


async def fetch_pulumi_state() -> dict[str, Any]:
    """Read Pulumi state file and return resource counts."""
    try:
        path = Path(PULUMI_STATE)
        if not path.exists():
            return {"exists": False}
        text = await asyncio.to_thread(path.read_text)
        data = json.loads(text)
        resources = data.get("checkpoint", {}).get("latest", {}).get("resources", [])
        pending = data.get("checkpoint", {}).get("latest", {}).get("pending_operations") or []
        total = len(resources)
        created = sum(1 for r in resources if r.get("created"))
        return {"exists": True, "total": total, "created": created, "pending": len(pending)}
    except Exception:
        return {"exists": False}


async def fetch_configmap() -> dict[str, Any] | None:
    """Fetch the gensim-run-status configmap."""
    raw = await run_cmd(
        _kubectl_cmd(
            [
                "get",
                "configmap",
                "gensim-run-status",
                "-o",
                "jsonpath={.data.status}",
                "-n",
                "default",
            ]
        ),
        env=KUBECTL_ENV,
    )
    if not raw:
        return None
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        return None


async def fetch_logs() -> str:
    """Fetch recent orchestrator logs (or local episode log in --local mode)."""
    if LOCAL_MODE:
        try:
            path = Path(LOCAL_EPISODE_LOG)
            if not path.exists():
                return ""
            text = await asyncio.to_thread(path.read_text)
            # Return last 80 lines to match the kubectl tail behavior
            return "\n".join(text.splitlines()[-80:])
        except Exception:
            return ""
    return await run_cmd(
        [
            "kubectl",
            "logs",
            "job/gensim-orchestrator",
            "--tail=80",
            "-n",
            "default",
        ],
        env=KUBECTL_ENV,
    )


async def fetch_eks_status() -> str:
    """Fetch EKS cluster status via AWS CLI.

    Expects AWS credentials in the environment (run the TUI under aws-vault exec).
    """
    return await run_cmd(
        [
            "aws",
            "eks",
            "describe-cluster",
            "--name",
            EKS_CLUSTER_NAME,
            "--region",
            EKS_REGION,
            "--query",
            "cluster.status",
            "--output",
            "text",
        ],
        timeout=20,
    )


def _kubectl_cmd(args: list[str]) -> list[str]:
    """Build a kubectl command, injecting --context for local mode."""
    cmd = ["kubectl"]
    if LOCAL_MODE:
        cmd += ["--context", f"kind-{_LOCAL_CLUSTER}"]
    return cmd + args


async def fetch_node_pod_counts() -> tuple[str, str]:
    """Fetch node and pod ready counts."""
    nodes_raw = await run_cmd(
        _kubectl_cmd(["get", "nodes", "-o", "json"]),
        env=KUBECTL_ENV,
    )
    pods_raw = await run_cmd(
        _kubectl_cmd(["get", "pods", "-A", "-o", "json"]),
        env=KUBECTL_ENV,
    )

    node_ready = node_total = 0
    if nodes_raw:
        try:
            items = json.loads(nodes_raw).get("items", [])
            node_total = len(items)
            for n in items:
                for c in n.get("status", {}).get("conditions", []):
                    if c.get("type") == "Ready" and c.get("status") == "True":
                        node_ready += 1
        except (json.JSONDecodeError, KeyError):
            pass

    pod_ready = pod_total = 0
    if pods_raw:
        try:
            items = json.loads(pods_raw).get("items", [])
            pod_total = len(items)
            for p in items:
                phase = p.get("status", {}).get("phase", "")
                if phase in ("Running", "Succeeded"):
                    pod_ready += 1
        except (json.JSONDecodeError, KeyError):
            pass

    return f"{node_ready}/{node_total}", f"{pod_ready}/{pod_total}"


async def fetch_pod_list() -> list[dict[str, str]]:
    """Fetch pod names and statuses in default namespace using lightweight output."""
    raw = await run_cmd(
        _kubectl_cmd(
            [
                "get",
                "pods",
                "-n",
                "default",
                "--no-headers",
                "-o",
                "custom-columns=NAME:.metadata.name,STATUS:.status.phase,READY:.status.containerStatuses[*].ready,RESTARTS:.status.containerStatuses[*].restartCount",
            ]
        ),
        env=KUBECTL_ENV,
    )
    if not raw:
        return []
    pods = []
    for line in raw.splitlines():
        parts = line.split()
        if len(parts) < 2:
            continue
        name = parts[0]
        phase = parts[1]
        # Parse ready: "true,true,true" -> 3/3
        ready_str = parts[2] if len(parts) > 2 else "<none>"
        if ready_str == "<none>":
            ready = "0/0"
        else:
            vals = ready_str.split(",")
            ready = f"{sum(1 for v in vals if v == 'true')}/{len(vals)}"
        # Parse restarts: "0,0,0" -> 0
        restart_str = parts[3] if len(parts) > 3 else "0"
        restarts = sum(int(r) for r in restart_str.split(",") if r.isdigit())
        pods.append({"name": name, "phase": phase, "ready": ready, "restarts": restarts})
    return pods


# ---------------------------------------------------------------------------
# Log parsing helpers
# ---------------------------------------------------------------------------

_PHASE_RE = re.compile(r"---\s+([\w]+)\s+phase(?:\s+\((\d+)s\))?\s+---", re.IGNORECASE)
_MONITOR_RE = re.compile(r"Monitor\s+\d+\s+status:\s+\S+\s+\((\d+)s\s+remaining\)")


def parse_phase_from_logs(logs: str) -> tuple[str, int | None]:
    """Return (current_phase, remaining_seconds) from the most recent log lines."""
    phase = ""
    remaining: int | None = None
    for line in logs.splitlines():
        m = _PHASE_RE.search(line)
        if m:
            phase = m.group(1).lower()
            remaining = None
        m2 = _MONITOR_RE.search(line)
        if m2:
            remaining = int(m2.group(1))
    return phase, remaining


# ---------------------------------------------------------------------------
# Formatting helpers
# ---------------------------------------------------------------------------


def _elapsed(started_at: str) -> str:
    """Human-readable elapsed time from an ISO timestamp to now."""
    try:
        start = datetime.fromisoformat(started_at.replace("Z", "+00:00"))
        delta = datetime.now(timezone.utc) - start
        total = int(delta.total_seconds())
        if total < 0:
            return "0s"
        h, rem = divmod(total, 3600)
        m, s = divmod(rem, 60)
        if h:
            return f"{h}h{m:02d}m{s:02d}s"
        if m:
            return f"{m}m{s:02d}s"
        return f"{s}s"
    except Exception:
        return "?"


def _progress_bar(fraction: float, width: int = 20) -> str:
    filled = int(fraction * width)
    filled = max(0, min(width, filled))
    return "\u2588" * filled + "\u2591" * (width - filled)


def _truncate(s: str, maxlen: int) -> str:
    return s if len(s) <= maxlen else s[: maxlen - 1] + "\u2026"


# ---------------------------------------------------------------------------
# Markup builders (pure functions returning markup strings)
# ---------------------------------------------------------------------------


def build_episode_markup(
    cm: dict[str, Any] | None,
    phase: str,
    remaining: int | None,
) -> str:
    if cm is None:
        if LOCAL_MODE:
            return (
                "[bold]No active local run detected.[/bold]\n"
                "\n"
                "Start one with:\n"
                "  [bold cyan]dda inv q.run-local-episode \\\\[/bold cyan]\n"
                "    [cyan]--episode=food-delivery-redis \\\\[/cyan]\n"
                "    [cyan]--image=<agent-image> \\\\[/cyan]\n"
                "    [cyan]--mode=live-and-record[/cyan]\n"
                "\n"
                "[dim]This TUI will auto-refresh when a run starts.[/dim]"
            )
        return (
            "[bold]No active gensim run detected.[/bold]\n"
            "\n"
            "Start one with:\n"
            "  [bold cyan]dda inv aws.eks.gensim.submit \\\\[/bold cyan]\n"
            "    [cyan]--image=<agent-image> \\\\[/cyan]\n"
            "    [cyan]--episodes=<episode:scenario> \\\\[/cyan]\n"
            "    [cyan]--mode=record-parquet[/cyan]\n"
            "\n"
            "Other commands:\n"
            "  [dim]dda inv aws.eks.gensim.status[/dim]   Quick status\n"
            "  [dim]dda inv aws.eks.gensim.destroy[/dim]  Tear down cluster\n"
            "  [dim]dda inv aws.eks.gensim.logs[/dim]     Orchestrator logs\n"
            "\n"
            "[dim]This TUI will auto-refresh when a run starts.[/dim]"
        )

    lines: list[str] = []
    lines.append("[bold]Episode Plan[/bold]")
    lines.append("")
    run_id = cm.get("runId", "?")
    image = cm.get("image", "?")
    if "/" in image:
        image = image.rsplit("/", 1)[-1]
    sha = cm.get("gensimSha", "")[:10]
    started = cm.get("startedAt", "")
    elapsed = _elapsed(started) if started else "?"
    completed = cm.get("completedAt")

    lines.append(f"  Run: [bold]{run_id}[/bold]")
    lines.append(f"  Image: {_truncate(image, 40)}")
    if sha:
        lines.append(f"  Gensim SHA: {sha}")
    if completed:
        lines.append(f"  Completed: {completed}")
        lines.append(f"  Total duration: {elapsed}")
    else:
        lines.append(f"  Elapsed: {elapsed}")
    lines.append("")

    episodes = cm.get("episodes", [])
    for i, ep in enumerate(episodes, 1):
        name = ep.get("episode", "?")
        scenario = ep.get("scenario", "")
        status = ep.get("status", "unknown")
        ep_phase = ep.get("phase", "")

        if status == "done":
            color, icon = "green", "[green]\u2713[/green]"
        elif status == "running":
            color, icon = "yellow", "[yellow]\u25b6[/yellow]"
        elif status == "failed":
            color, icon = "red", "[red]\u2717[/red]"
        else:
            color, icon = "dim", "[dim]\u25cb[/dim]"

        short_name = _truncate(name, 30)
        short_scenario = _truncate(scenario, 25)
        lines.append(f"  {icon} [{color}]\\[{i}] {short_name} / {short_scenario}[/{color}]")
        lines.append(f"      Status: [{color}]{status}[/{color}]")

        if status == "running":
            display_phase = ep_phase or phase or "?"
            lines.append(f"      Phase: {display_phase}")
            if remaining is not None and phase:
                total_dur = PHASE_DURATIONS.get(phase)
                if total_dur:
                    elapsed_in_phase = total_dur - remaining
                    frac = max(0.0, min(1.0, elapsed_in_phase / total_dur))
                    bar = _progress_bar(frac)
                    lines.append(f"      Progress: {bar} {remaining}s left")
                else:
                    lines.append(f"      Remaining: {remaining}s")
        lines.append("")

    done = sum(1 for e in episodes if e.get("status") == "done")
    total = len(episodes)
    lines.append(f"  Episodes: {done}/{total} complete")

    return "\n".join(lines)


def build_infra_markup(
    pulumi: dict[str, Any],
    eks_status: str,
    node_counts: str,
    pod_counts: str,
) -> str:
    lines: list[str] = []
    lines.append("[bold]Infra Status[/bold]")
    lines.append("")

    eks = eks_status or "unknown"
    if eks.upper() == "ACTIVE":
        lines.append(f"  EKS: [green]{eks}[/green]")
    elif eks == "unknown":
        lines.append("  EKS: [dim]unknown[/dim]")
    else:
        lines.append(f"  EKS: [yellow]{eks}[/yellow]")

    lines.append(f"  Nodes: {node_counts}")
    lines.append(f"  Pods:  {pod_counts}")
    lines.append("")

    if pulumi.get("exists"):
        total = pulumi.get("total", 0)
        created = pulumi.get("created", 0)
        pending = pulumi.get("pending", 0)
        if total > 0:
            frac = created / total
            bar = _progress_bar(frac, 15)
            lines.append(f"  Pulumi: {created}/{total}")
            lines.append(f"  {bar}")
        else:
            lines.append("  Pulumi: 0 resources")
        if pending:
            lines.append(f"  [yellow]Pending: {pending}[/yellow]")
    else:
        lines.append("  Pulumi: [dim]no state[/dim]")

    return "\n".join(lines)


def build_pods_markup(pods: list[dict[str, str]]) -> str:
    if not pods:
        return "[bold]Pods[/bold]\n[dim]No pods in default namespace[/dim]"
    lines: list[str] = ["[bold]Pods[/bold]", ""]
    for p in pods:
        name = _truncate(p["name"], 38)
        phase = p["phase"]
        ready = p["ready"]
        restarts = p["restarts"]
        if phase == "Running" and ready.split("/")[0] == ready.split("/")[1]:
            color = "green"
        elif phase in ("Succeeded", "Completed"):
            color = "dim"
        elif phase == "Running":
            color = "yellow"
        else:
            color = "red"
        restart_str = f" [red]R:{restarts}[/red]" if restarts else ""
        lines.append(f"  [{color}]{name}[/{color}]")
        lines.append(f"    [{color}]{phase}[/{color}] {ready}{restart_str}")
    return "\n".join(lines)


def build_log_markup(log_text: str) -> str:
    if not log_text:
        return "[bold]Orchestrator Logs[/bold]\n[dim]No orchestrator logs yet...[/dim]"

    tail = log_text.strip().splitlines()[-20:]
    colored: list[str] = []
    for line in tail:
        # Escape markup characters in log lines to prevent rendering issues
        safe = line.replace("[", "\\[")
        if "---" in line and "phase" in line.lower():
            colored.append(f"[bold cyan]> {safe}[/bold cyan]")
        elif "Monitor" in line and "status:" in line:
            if "Alert" in line:
                colored.append(f"[bold red]> {safe}[/bold red]")
            elif "OK" in line:
                colored.append(f"[bold green]> {safe}[/bold green]")
            else:
                colored.append(f"[yellow]> {safe}[/yellow]")
        elif "error" in line.lower() or "failed" in line.lower():
            colored.append(f"[red]> {safe}[/red]")
        elif "completed" in line.lower() or "success" in line.lower():
            colored.append(f"[green]> {safe}[/green]")
        else:
            colored.append(f"[dim]> {safe}[/dim]")
    return "[bold]Orchestrator Logs[/bold]\n" + "\n".join(colored)


# ---------------------------------------------------------------------------
# Widgets -- use reactive to drive updates and avoid attribute name collisions
# ---------------------------------------------------------------------------


class EpisodePlanWidget(Static):
    """Left panel: run metadata + episode list."""

    DEFAULT_CSS = """
    EpisodePlanWidget {
        width: 1fr;
        height: 100%;
        padding: 1 2;
    }
    """

    content_text: reactive[str] = reactive("[dim]Waiting for configmap...[/dim]", layout=True)

    def watch_content_text(self, value: str) -> None:
        self.update(value)


class InfraWidget(Static):
    """Right panel: infrastructure status."""

    DEFAULT_CSS = """
    InfraWidget {
        width: 30;
        height: 100%;
        padding: 1 2;
        border-left: solid $surface-lighten-2;
    }
    """

    content_text: reactive[str] = reactive("[bold]Infra Status[/bold]\n\n  [dim]Loading...[/dim]", layout=True)

    def watch_content_text(self, value: str) -> None:
        self.update(value)


class PodsWidget(Static):
    """Right panel: live pod list."""

    DEFAULT_CSS = """
    PodsWidget {
        width: 45;
        height: 100%;
        padding: 1 2;
        border-left: solid $surface-lighten-2;
    }
    """

    content_text: reactive[str] = reactive("[bold]Pods[/bold]\n\n  [dim]Loading...[/dim]", layout=True)

    def watch_content_text(self, value: str) -> None:
        self.update(value)


class LogWidget(Static):
    """Bottom panel: orchestrator log tail."""

    DEFAULT_CSS = """
    LogWidget {
        height: 12;
        padding: 0 2;
        border-top: solid $surface-lighten-2;
    }
    """

    content_text: reactive[str] = reactive(
        "[bold]Orchestrator Logs[/bold]\n[dim]No orchestrator logs yet...[/dim]",
        layout=True,
    )

    def watch_content_text(self, value: str) -> None:
        self.update(value)


class PodLogsWidget(Static):
    """Streaming pod logs panel, only visible when terminal is tall enough."""

    DEFAULT_CSS = """
    PodLogsWidget {
        height: 14;
        padding: 0 2;
        border-top: solid $surface-lighten-2;
        display: none;
    }
    """

    content_text: reactive[str] = reactive(
        "[bold]Pod Logs[/bold]\n[dim]Waiting for pods...[/dim]",
        layout=True,
    )
    _stream_task: asyncio.Task | None = None
    _render_task: asyncio.Task | None = None
    _log_lines: list[str] = []
    _max_lines: int = 30
    _dirty: bool = False
    _target: str = ""

    def watch_content_text(self, value: str) -> None:
        self.update(value)

    def start_streaming(self) -> None:
        """Start the kubectl log streaming subprocess and render timer."""
        if self._stream_task is not None:
            return
        self._log_lines = []
        self._stream_task = asyncio.ensure_future(self._stream_logs())
        self._render_task = asyncio.ensure_future(self._render_loop())

    def stop_streaming(self) -> None:
        """Stop the streaming subprocess and render timer."""
        if self._stream_task is not None:
            self._stream_task.cancel()
            self._stream_task = None
        if self._render_task is not None:
            self._render_task.cancel()
            self._render_task = None

    async def _render_loop(self) -> None:
        """Flush buffered log lines to the UI at a fixed rate (2Hz)."""
        try:
            while True:
                await asyncio.sleep(0.5)
                if not self._dirty:
                    continue
                self._dirty = False
                safe_lines = [ln.replace("[", "\\[") for ln in self._log_lines[-12:]]
                self.content_text = f"[bold]Pod Logs[/bold] [dim](streaming {self._target})[/dim]\n" + "\n".join(
                    f"[dim]> {ln}[/dim]" for ln in safe_lines
                )
        except asyncio.CancelledError:
            return

    async def _stream_logs(self) -> None:
        """Stream kubectl logs from episode pods, auto-retry on failure."""
        while True:
            exclude = {
                "datadog-agent",
                "datadog-agent-cluster-agent",
                "datadog-agent-operator",
                "gensim-orchestrator",
            }
            pods_raw = await run_cmd(
                ["kubectl", "get", "pods", "-n", "default", "-o", "jsonpath={.items[*].metadata.name}"],
                env=KUBECTL_ENV,
            )
            pod_names = [p for p in pods_raw.split() if p and not any(p.startswith(ex) for ex in exclude)]

            if not pod_names:
                self.content_text = "[bold]Pod Logs[/bold]\n[dim]No episode pods running...[/dim]"
                await asyncio.sleep(10)
                continue

            self._target = pod_names[0]
            self.content_text = f"[bold]Pod Logs[/bold] [dim](connecting to {self._target})[/dim]"
            cmd = [
                "kubectl",
                "logs",
                "-f",
                "--all-containers=true",
                "--tail=10",
                "-n",
                "default",
                self._target,
            ]
            try:
                proc = await asyncio.create_subprocess_exec(
                    *cmd,
                    stdout=asyncio.subprocess.PIPE,
                    stderr=asyncio.subprocess.DEVNULL,
                    env=KUBECTL_ENV,
                )
                while True:
                    line = await asyncio.wait_for(proc.stdout.readline(), timeout=30)
                    if not line:
                        break
                    decoded = line.decode("utf-8", errors="replace").rstrip()
                    if not decoded:
                        continue
                    self._log_lines.append(decoded)
                    if len(self._log_lines) > self._max_lines:
                        self._log_lines = self._log_lines[-self._max_lines :]
                    self._dirty = True
            except asyncio.CancelledError:
                return
            except (TimeoutError, Exception):
                pass

            self.content_text = "[bold]Pod Logs[/bold]\n[dim]Stream ended, retrying...[/dim]"
            await asyncio.sleep(5)


# ---------------------------------------------------------------------------
# App
# ---------------------------------------------------------------------------


class GensimStatusApp(App):
    """Gensim EKS evaluation run monitor."""

    CSS = """
    #top-row {
        height: 1fr;
    }
    """

    TITLE = "Gensim Local Monitor" if LOCAL_MODE else "Gensim EKS Monitor"
    BINDINGS = [
        Binding("q", "quit", "Quit"),
        Binding("r", "refresh", "Refresh now"),
    ]

    def __init__(self, **kwargs: Any) -> None:
        super().__init__(**kwargs)
        # Shared state updated by pollers, read by renderers
        self._cm_data: dict[str, Any] | None = None
        self._phase: str = ""
        self._remaining: int | None = None
        self._pulumi_data: dict[str, Any] = {}
        self._eks_status: str = ""
        self._node_counts: str = "?"
        self._pod_counts: str = "?"
        self._pod_list: list[dict[str, str]] = []
        self._eks_failures: int = 0  # consecutive EKS poll failures for backoff

    # Minimum terminal height to show the pod logs panel
    _POD_LOGS_MIN_HEIGHT = 40

    def compose(self) -> ComposeResult:
        yield Header()
        with Horizontal(id="top-row"):
            yield EpisodePlanWidget()
            yield InfraWidget()
            yield PodsWidget()
        yield LogWidget()
        yield PodLogsWidget()
        yield Footer()

    def on_mount(self) -> None:
        self.call_later(self.action_refresh)
        if not LOCAL_MODE:
            self.set_interval(5, self._poll_pulumi)
            self.set_interval(60, self._poll_eks)
        self.set_interval(10, self._poll_configmap)
        self.set_interval(5, self._poll_logs)
        self.set_interval(30, self._poll_infra_kube)
        self.set_interval(30, self._poll_pods)
        self._update_pod_logs_visibility()

    def on_resize(self) -> None:
        self._update_pod_logs_visibility()

    def _update_pod_logs_visibility(self) -> None:
        """Show/hide pod logs panel based on terminal height, start/stop stream."""
        panel = self.query_one(PodLogsWidget)
        should_show = self.size.height >= self._POD_LOGS_MIN_HEIGHT
        if should_show and not panel.display:
            panel.display = True
            panel.start_streaming()
        elif not should_show and panel.display:
            panel.stop_streaming()
            panel.display = False

    async def action_refresh(self) -> None:
        """Manual refresh all data sources."""
        await asyncio.gather(
            self._poll_pulumi(),
            self._poll_configmap(),
            self._poll_logs(),
            self._poll_infra_kube(),
            self._poll_pods(),
            self._poll_eks(),
        )

    async def _poll_pulumi(self) -> None:
        self._pulumi_data = await fetch_pulumi_state()
        self._refresh_infra()

    async def _poll_configmap(self) -> None:
        self._cm_data = await fetch_configmap()
        self._refresh_episodes()

    async def _poll_logs(self) -> None:
        logs = await fetch_logs()
        self.query_one(LogWidget).content_text = build_log_markup(logs)
        if logs:
            self._phase, self._remaining = parse_phase_from_logs(logs)
            self._refresh_episodes()

    async def _poll_infra_kube(self) -> None:
        self._node_counts, self._pod_counts = await fetch_node_pod_counts()
        self._refresh_infra()

    async def _poll_pods(self) -> None:
        self._pod_list = await fetch_pod_list()
        self.query_one(PodsWidget).content_text = build_pods_markup(self._pod_list)

    async def _poll_eks(self) -> None:
        # Exponential backoff: skip polls after consecutive failures to avoid
        # hammering AWS auth (which can open browser tabs on SSO expiry).
        if self._eks_failures >= 3:
            skip_rounds = min(2 ** (self._eks_failures - 3), 16)
            if not hasattr(self, "_eks_skip_count"):
                self._eks_skip_count = 0
            self._eks_skip_count += 1
            if self._eks_skip_count < skip_rounds:
                return
            self._eks_skip_count = 0

        result = await fetch_eks_status()
        if result:
            self._eks_status = result
            self._eks_failures = 0
        else:
            self._eks_failures += 1
            if self._eks_failures == 3:
                self._eks_status = "auth-error (backoff)"
        self._refresh_infra()

    def _refresh_episodes(self) -> None:
        markup = build_episode_markup(self._cm_data, self._phase, self._remaining)
        self.query_one(EpisodePlanWidget).content_text = markup

    def _refresh_infra(self) -> None:
        markup = build_infra_markup(
            self._pulumi_data,
            self._eks_status,
            self._node_counts,
            self._pod_counts,
        )
        self.query_one(InfraWidget).content_text = markup


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    headless = "--headless" in sys.argv
    # Strip custom flags before Textual sees argv
    sys.argv = [a for a in sys.argv if a not in ("--local", "--headless")]
    app = GensimStatusApp()
    app.run(headless=headless)
