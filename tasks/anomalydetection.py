"""
Invoke tasks for anomaly detection dev tooling (not part of agent build).
"""

import shlex

from invoke import task

from tasks.libs.common.color import Color, color_message

# --- Build ---


@task
def build_testbench(ctx):
    """
    Builds the anomalydetection-testbench binary to bin/anomalydetection-testbench.
    """
    ctx.run(
        "go build -C internal/qbranch/anomalydetection-testbench -tags python -o ../../../bin/anomalydetection-testbench ."
    )


# --- Run ---


ALL_DETECTORS = "cusum,bocpd,rrcf,scanmw,scanwelch,holt_residual,tukey_biweight"


@task
def launch_testbench(
    ctx,
    scenarios_dir: str = "./comp/anomalydetection/observer/scenarios",
    build: bool = False,
    headless_scenario: str = "",
    headless_output: str = "",
    profile: bool = False,
    open_pprof: bool = False,
    verbose: bool = False,
    profile_path: str = "",
    config: str = "",
    enable: str = "",
    disable: str = "",
    timeout: int = 0,
    logs_only: bool = False,
    detectors_only: bool = False,
):
    """
    Launches the anomalydetection-testbench backend (and UI in interactive mode).

    Args:
        scenarios_dir: Directory containing the scenarios to load.
        build: Whether to build the binary before launching.
        profile: Whether to capture a heap profile (headless mode only).
        open_pprof: Open pprof UI after headless run (requires --profile).
        verbose: Pass --verbose to the testbench.
        profile_path: Override the default heap-profile output path.
        config: JSON params file; overrides --enable/--disable when set.
        enable: Comma-separated components to enable (passed as --enable).
        disable: Comma-separated components to disable (passed as --disable).
        timeout: Kill the headless process after this many seconds (0 = no limit).
        logs_only: Pass --logs-only (skip parquet metrics and trace stats).
        detectors_only: Enable ALL detectors and disable ALL correlators (no --config/--enable/--disable override).
    """
    if build:
        print("Building anomalydetection-testbench...")
        build_testbench(ctx)

    flags = ""
    if verbose:
        flags += " --verbose"
    if logs_only:
        flags += " --logs-only"
    if config:
        flags += f" --config {shlex.quote(config)}"
    else:
        if enable:
            flags += f" --enable {shlex.quote(enable)}"
        if disable:
            flags += f" --disable {shlex.quote(disable)}"

    if headless_scenario:
        if not headless_output:
            headless_output = f"/tmp/anomalydetection-testbench-headless-{headless_scenario}.json"
        if profile:
            if not profile_path:
                profile_path = f"/tmp/anomalydetection-testbench-headless-{headless_scenario}.prof"
            flags += f" --memprofile {profile_path}"
        print(
            f"Launching anomalydetection-testbench in headless mode for scenario {headless_scenario}, output to {headless_output}"
        )
        try:
            ctx.run(
                f"bin/anomalydetection-testbench --headless {headless_scenario} --scenarios-dir {scenarios_dir} --output {headless_output}{flags}",
                timeout=None if timeout == 0 else timeout,
            )
        except Exception as e:
            if type(e).__name__ == "CommandTimedOut":
                print(color_message(f"testbench timed out after {timeout}s", Color.ORANGE))
            else:
                raise
        if profile:
            if open_pprof:
                print("Running pprof...")
                ctx.run(f"go tool pprof -http=:8081 {profile_path}")
            else:
                print(f"To profile, run: go tool pprof -http=:8081 {profile_path}")
    else:
        if not config and not enable and not disable:
            if detectors_only:
                flags += f" --only {ALL_DETECTORS}"
                print(f"Launching with all detectors, no correlators: {ALL_DETECTORS}")
            else:
                flags += " --only scanmw,scanwelch,bocpd"
        print("Launching anomalydetection-testbench backend and UI, use ^C to exit")
        print(
            "To profile, run: go tool pprof -http=:8081 http://localhost:8080/debug/pprof/heap (8080 is the testbench API port)"
        )
        ctx.run(
            f"bin/anomalydetection-testbench --scenarios-dir {scenarios_dir}{flags} & ( cd internal/qbranch/anomalydetection-testbench/ui && npm install && npm run dev ) &"
        )
