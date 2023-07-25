"""
SMP namespaced tasks
"""


from .agent import build, BIN_PATH
from .flavor import AgentFlavor
from invoke import task
from .utils import (
    bin_name,
)
import os


@task
def run_regression(
    ctx,
    rebuild=False,
    race=False,
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
    skip_build=False,
    regression_case="uds_dogstatsd_to_api",
    run_telemetry_agent=True,
):
    """
    Run the specified regression test against the locally built agent.

    By default it builds the agent before executing it, unless --skip-build was
    passed. It accepts the same set of options as agent.build.
    """
    if not skip_build:
        build(ctx, rebuild, race, build_include, build_exclude, flavor)

    agent_bin = os.path.join(BIN_PATH, bin_name("agent"))

    # TODO attempt to run  `cargo install --git https://github.com/DataDog/lading/ lading`
    # Would be nice to support custom lading version for development, either custom git version or directory on disk
    # Detect errors with incorrect rust version and offer advice to update/install via rustup

    regression_test_dir = os.path.join(".", "test", "regression")
    telemetry_agent_name = "agnt-smp-regression-localrun" # TODO maybe add random ID to avoid collisions

    dd_api_key_set = os.environ.get("DD_API_KEY") is not None
    if not dd_api_key_set:
        print("Warn: $DD_API_KEY not set, not running telemetry agent")

    if run_telemetry_agent and dd_api_key_set:
        # Run with inherit environment so that $DD_API_KEY is available
        openmetrics_confd = os.path.join(regression_test_dir, "local-telemetry-agent-confd", "openmetrics.d")

        # TODO turn off more componenst of the agent.
        # Only want to be running:
        # - python openmetrics checks
        # - trace-agent listening on 8126
        # CMD_PORT and EXPVAR_PORT are set to arbitrary values that are unlikely to conflict
        telemetry_agent_docker_cmd = f"docker run -d --rm --name {telemetry_agent_name} -e DD_CMD_PORT=8008 -e DD_EXPVAR_PORT=8009 -v {openmetrics_confd}:/etc/datadog-agent/conf.d/openmetrics.d -v /var/run/docker.sock:/var/run/docker.sock:ro -v /proc/:/host/proc/:ro -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro --network host datadog/agent"
        print("Running dockerized telemetry agent configured to scrape lading/agent metrics")
        ctx.run(telemetry_agent_docker_cmd)
        # TODO this doesn't work yet


    lading_cmd = f"lading --target-path {agent_bin} --config-path {regression_test_dir}/cases/{regression_case}/lading/lading.yaml -- -c {regression_test_dir}/cases/{regression_case}/datadog-agent/datadog.yaml run"
    lading_env = {'DD_HOSTNAME': 'smp-regression-local', 'RUST_LOG': "lading=debug,lading::blackhole::http=warn"}

    print(f"Running lading regression experiment locally in the background. full cmd: {lading_cmd}")
    ctx.run(lading_cmd, env=lading_env)

    if run_telemetry_agent:
        ctx.run(f"docker stop {telemetry_agent_name}")



