"""
Build, test and run localdog, a local Datadog backend for the agent (test/localdog)
"""

import os
import shutil

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.libs.common.go import go_build

LOCALDOG_DIR = "test/localdog"
BIN_PATH = os.path.join(LOCALDOG_DIR, "build", "localdog")
EMBEDDED_UI_DIR = os.path.join(LOCALDOG_DIR, "server", "ui")
DEFAULT_WEB_UI_PATH = os.path.join("..", "web-ui")
WEB_UI_APP_DIR = os.path.join("static-apps", "localdog")


def _web_ui_dist(web_ui_path):
    return os.path.join(web_ui_path, WEB_UI_APP_DIR, "dist")


@task(
    help={
        "with_ui": "Embed the localdog web app in the binary (see build-ui)",
        "web_ui_path": f"Path to a web-ui checkout (default: {DEFAULT_WEB_UI_PATH})",
        "skip_ui_build": "Embed the existing web app build instead of rebuilding it",
    }
)
def build(ctx, with_ui=False, web_ui_path=DEFAULT_WEB_UI_PATH, skip_ui_build=False):
    """
    Build localdog to test/localdog/build/localdog
    """
    build_tags = []
    if with_ui:
        if not skip_ui_build:
            build_ui(ctx, web_ui_path=web_ui_path)
        dist = _web_ui_dist(web_ui_path)
        if not os.path.isfile(os.path.join(dist, "index.html")):
            raise Exit(f"No web app build found in {dist}, run `dda inv localdog.build-ui` first", code=1)
        shutil.rmtree(EMBEDDED_UI_DIR, ignore_errors=True)
        shutil.copytree(dist, EMBEDDED_UI_DIR, ignore=shutil.ignore_patterns("*.map", "_private"))
        build_tags.append("localdog_ui")
    with ctx.cd(LOCALDOG_DIR):
        go_build(ctx, "./cmd/localdog", build_tags=build_tags, bin_path=os.path.join("build", "localdog"))
    print(color_message(f"localdog built at {BIN_PATH}", "green"))


@task(help={"web_ui_path": f"Path to a web-ui checkout (default: {DEFAULT_WEB_UI_PATH})"})
def build_ui(ctx, web_ui_path=DEFAULT_WEB_UI_PATH):
    """
    Build the localdog web app (web-ui static-apps/localdog)
    """
    if not os.path.isdir(os.path.join(web_ui_path, WEB_UI_APP_DIR)):
        raise Exit(f"{WEB_UI_APP_DIR} not found in web-ui checkout {web_ui_path}", code=1)
    with ctx.cd(web_ui_path):
        ctx.run("yarn install")
        ctx.run(
            "yarn cli static-apps build --app=localdog --env=prod --branch=localdog --version=0.0.1",
            env={"NODE_OPTIONS": "--max-old-space-size=8192"},
        )


@task
def test(ctx):
    """
    Run the localdog tests
    """
    with ctx.cd(LOCALDOG_DIR):
        ctx.run("go test ./...")


@task(
    help={
        "addr": "Address to listen on",
        "ui_dir": "Serve the web app from this directory (default: the web-ui build if present)",
        "web_ui_path": f"Path to a web-ui checkout (default: {DEFAULT_WEB_UI_PATH})",
        "data_dir": "Directory where data is persisted (default: ~/.localdog)",
    }
)
def run(ctx, addr="127.0.0.1:8282", ui_dir=None, web_ui_path=DEFAULT_WEB_UI_PATH, data_dir=None):
    """
    Build and run localdog
    """
    build(ctx)
    if ui_dir is None and os.path.isfile(os.path.join(_web_ui_dist(web_ui_path), "index.html")):
        ui_dir = os.path.abspath(_web_ui_dist(web_ui_path))
    args = [f"-addr {addr}"]
    if ui_dir:
        args.append(f"-ui-dir {ui_dir}")
    if data_dir is not None:
        args.append(f"-data-dir {data_dir}")
    ctx.run(f"{BIN_PATH} {' '.join(args)}", pty=True)


@task(
    help={
        "url": "localdog URL the agent should send to",
        "log_dir": "Directory of the demo app logs, tailed by the agent",
    }
)
def agent(ctx, url="http://localhost:8282", log_dir=None):
    """
    Start a Datadog Agent container (localdog-agent) that sends everything to localdog
    """
    env = {"LOCALDOG_URL": url}
    if log_dir:
        env["LOG_DIR"] = log_dir
    ctx.run(os.path.join(LOCALDOG_DIR, "demo", "run-agent.sh"), env=env)


@task(help={"log_dir": "Directory where the demo app writes its logs"})
def demo(ctx, log_dir=None):
    """
    Start the demo Node.js app, instrumented to send traces, logs and metrics to the agent
    """
    env = {}
    if log_dir:
        env["LOG_DIR"] = log_dir
    ctx.run(os.path.join(LOCALDOG_DIR, "demo", "run-demo.sh"), env=env)


@task
def stop(ctx):
    """
    Stop the demo app and the localdog-agent container
    """
    ctx.run(os.path.join(LOCALDOG_DIR, "demo", "stop-demo.sh"), warn=True)
    ctx.run(os.path.join(LOCALDOG_DIR, "demo", "stop-agent.sh"), warn=True)


def _post_json(url, body):
    import json
    import urllib.request

    req = urllib.request.Request(url, data=json.dumps(body).encode(), headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=10) as resp:
        return json.load(resp)


def _get_json(url):
    import json
    import urllib.request

    with urllib.request.urlopen(url, timeout=10) as resp:
        return json.load(resp)


@task(
    help={
        "url": "localdog URL",
        "service": "Service whose telemetry must be present",
        "timeout": "Seconds to wait for data",
    }
)
def check(_, url="http://localhost:8282", service="demo-node", timeout=180):
    """
    Check end to end that localdog received logs, traces and metrics from the agent for a service,
    and that the web app APIs return them (logs correlate with traces)
    """
    import time

    deadline = time.time() + int(timeout)
    checks = {}

    def run_checks():
        now = int(time.time() * 1000)
        window = {"from": now - 15 * 60 * 1000, "to": now}
        logs = _post_json(
            f"{url}/api/v1/logs-analytics/list?type=logs",
            {"list": {"limit": 50, "time": window, "search": {"query": f"service:{service}"}}},
        )["result"]["events"]
        checks["logs"] = len(logs) > 0
        spans = _post_json(
            f"{url}/api/v1/logs-analytics/list?type=trace",
            {"list": {"limit": 50, "time": window, "search": {"query": f"service:{service}"}}},
        )["result"]["events"]
        checks["spans"] = len(spans) > 0
        correlated = [e["event"] for e in logs if e["event"].get("trace_id")]
        checks["log-trace correlation"] = False
        for log in correlated[:20]:
            try:
                trace = _get_json(f"{url}/api/ui/trace/{log['trace_id']}")
            except Exception:
                continue
            if trace["trace"]["spans"]:
                checks["log-trace correlation"] = True
                break
        ts = _post_json(
            f"{url}/api/ui/query/timeseries",
            {
                "data": [
                    {
                        "type": "timeseries_request",
                        "attributes": {
                            **window,
                            "interval": 60000,
                            "formulas": [{"formula": "query1"}, {"formula": "query2"}],
                            "queries": [
                                {"data_source": "metrics", "name": "query1", "query": "avg:system.cpu.user{*}"},
                                {
                                    "data_source": "metrics",
                                    "name": "query2",
                                    "query": f"sum:trace.express.request.hits{{service:{service}}}.as_count()",
                                },
                            ],
                        },
                    }
                ]
            },
        )["data"][0]["attributes"]
        values = [v for series in ts["values"] for v in series if v is not None]
        checks["agent metrics"] = any(s["query_index"] == 0 for s in ts["series"]) and len(values) > 0
        metric_names = [m["name"] for m in _get_json(f"{url}/api/localdog/metrics")["metrics"]]
        checks["custom metrics"] = any(not n.startswith(("system.", "datadog.", "trace.")) for n in metric_names)
        return all(checks.values())

    while True:
        try:
            if run_checks():
                break
        except Exception as e:  # localdog not up yet
            checks["localdog reachable"] = False
            print(color_message(f"waiting for localdog: {e}", "orange"))
        if time.time() > deadline:
            break
        time.sleep(5)
    for name, ok in checks.items():
        print(color_message(f"{'PASS' if ok else 'FAIL'} {name}", "green" if ok else "red"))
    if not checks or not all(checks.values()):
        raise Exit("localdog end-to-end check failed", code=1)
