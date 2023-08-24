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
import time
from pathlib import Path
import secrets
import shutil
import logging

ONE_SEC_IN_NS = 1_000_000_000
LOG_DIR = "smp-local-run-logs"

# Install non-released versions of lading via
# cargo install --rev=my-feature-branch --git https://github.com/DataDog/lading/ lading
# or --sha
def check_for_lading_binary(ctx):
    if shutil.which("lading") is None:
        print(f"'lading' is not found. Consider installing via by running 'cargo install --git https://github.com/DataDog/lading/ lading'")

def configure_logger():
    # create logger
    logger = logging.getLogger('smp-local-run')
    logger.setLevel(logging.DEBUG)

    # create console handler and set level to debug
    ch = logging.StreamHandler()
    ch.setLevel(logging.DEBUG)

    if not os.path.exists(LOG_DIR):
        os.makedirs(LOG_DIR)
    # create file handler and set level to debug
    fh = logging.FileHandler(f"{LOG_DIR}/run.log")
    fh.setLevel(logging.DEBUG)

    # create formatter
    formatter = logging.Formatter('%(asctime)s - %(name)s - %(levelname)s - %(message)s')

    # add formatter to ch and fh
    ch.setFormatter(formatter)
    fh.setFormatter(formatter)

    # add ch and fh to logger
    logger.addHandler(ch)
    logger.addHandler(fh)

    return logger



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
    enable_profiler=True,
    experiment_duration_seconds=120
):
    """
    Run the specified regression test against the locally built agent.

    By default it builds the agent before executing it, unless --skip-build was
    passed. It accepts the same set of options as agent.build.
    """
    if not skip_build:
        build(ctx, rebuild, race, build_include, build_exclude, flavor)

    telemetry_agent_name = "agnt-smp-regression-localrun"
    logger = configure_logger()
    run_id = secrets.token_hex(16)
    logger.info(f"Starting local run of scenario '{regression_case}'. Run ID for reference: {run_id}")

    try:
        agent_bin = os.path.join(BIN_PATH, bin_name("agent"))

        check_for_lading_binary(ctx)

        regression_test_dir = os.path.join(".", "test", "regression")

        dd_api_key_set = os.environ.get("DD_API_KEY") is not None
        if not dd_api_key_set:
            logger.warn("$DD_API_KEY not set, not running telemetry agent")

        lading_env = {'DD_HOSTNAME': 'smp-regression-local', 'RUST_LOG': "lading=debug,lading::blackhole::http=warn"}

        logs_dir = os.path.join(os.getcwd(), LOG_DIR)

        if run_telemetry_agent and dd_api_key_set:
            openmetrics_confd = os.path.join(regression_test_dir, "local-telemetry-agent-confd", "openmetrics.d")
            logs_confd = os.path.join(regression_test_dir, "local-telemetry-agent-confd", "logs.d")


            # TODO turn off more components of the agent.
            # Only want to be running:
            # - python openmetrics checks
            # - trace-agent listening on 8126
            # CMD_PORT and EXPVAR_PORT are set to arbitrary values that are unlikely to conflict
            telemetry_agent_docker_cmd = f"docker run -d --rm --name {telemetry_agent_name} -e DD_LOGS_ENABLED=true -e DD_CMD_PORT=8008 -e DD_EXPVAR_PORT=8009 -e DD_API_KEY=$DD_API_KEY -v {logs_confd}:/etc/datadog-agent/conf.d/logs.d -v {openmetrics_confd}:/etc/datadog-agent/conf.d/openmetrics.d -v /var/run/docker.sock:/var/run/docker.sock:ro -v /proc/:/host/proc/:ro -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro -v {logs_dir}:/mnt/smp-logs --network host datadog/agent"
            logger.info(f"Running dockerized telemetry agent configured to scrape lading/agent metrics. cmd: {telemetry_agent_docker_cmd}")
            ctx.run(telemetry_agent_docker_cmd)
            lading_env['DD_TELEMETRY_ENABLED'] = "true"
            if enable_profiler:
                lading_env['DD_INTERNAL_PROFILING_ENABLED'] = "true"
                user = os.getlogin()
                lading_env['DD_INTERNAL_PROFILING_EXTRA_TAGS'] = "[\"env:smp-local-run\", \"service:{regression_case}\", \"user:{user}\"]"
                # sets profiling period to half the total experiment duration to collect
                lading_env['DD_INTERNAL_PROFILING_PERIOD'] = str(experiment_duration_seconds * ONE_SEC_IN_NS)
                lading_env['DD_INTERNAL_PROFILING_CPU_DURATION'] = str(30 * ONE_SEC_IN_NS)
                experiment_duration_seconds *= 2

        start_ts = int(round(time.time() * 1000))
        lading_cmd = f"lading --target-path {agent_bin} --target-inherit-environment --target-stdout-path={logs_dir}/stdout.log --target-stderr-path={logs_dir}/stderr.log --experiment-duration-seconds {experiment_duration_seconds} --config-path {regression_test_dir}/cases/{regression_case}/lading/lading.yaml -- -c {regression_test_dir}/cases/{regression_case}/datadog-agent/datadog.yaml run"


        logger.info(f"Running lading regression experiment locally in the background. Duration is {experiment_duration_seconds} seconds. Full cmd: {lading_cmd}")

        agent_config_path = f"{regression_test_dir}/cases/{regression_case}/datadog-agent/datadog.yaml"
        agent_config_str = Path(agent_config_path).read_text()
        logger.info(f"===================== Start Agent Config =====================\n{agent_config_str}\n===================== End Agent Config =====================")

        lading_config_path = f"{regression_test_dir}/cases/{regression_case}/lading/lading.yaml"
        lading_config_str = Path(lading_config_path).read_text()
        logger.info(f"===================== Start Lading Config =====================\n{lading_config_str}\n===================== End Lading Config =====================")

        ctx.run(lading_cmd, env=lading_env)


        end_ts = int(round(time.time() * 1000))
        # This dashboard is uds/dogstatsd specific.
        # Future improvement would be to allow some yaml config in the regression_case dir to define the dashboard to run
        logger.info(f"Run completed! View results in https://app.datadoghq.com/dashboard/bvi-a7a-spq?from_ts={start_ts}&to_ts={end_ts}&live=false&tpl_var_zocf={run_id}  Run ID: {run_id}")
        logging.shutdown()

    finally:
        if run_telemetry_agent:
            time.sleep(5) # Give the telemetry agent time to fully read all logs
            ctx.run(f"docker stop {telemetry_agent_name}")



