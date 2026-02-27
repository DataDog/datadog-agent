import os
import random
import time

from invoke import task


# --- Build ---
@task
def build_testbench(ctx):
    """
    Builds the observer-testbench binary.
    """
    ctx.run("go build -o bin/observer-testbench ./cmd/observer-testbench")


@task
def launch_testbench(ctx, scenarios_dir: str = "./comp/observer/anomaly_datasets_converted", build: bool = False):
    """
    Will launch both the observer-testbench backend and UI.

    Args:
        scenarios_dir: The directory containing the scenarios to load.
        build: Whether to build the observer-testbench binary.
    """
    if build:
        print("Building observer-testbench...")
        build_testbench(ctx)

    print("Launching observer-testbench backend and UI, use ^C to exit")
    ctx.run(
        f"bin/observer-testbench --scenarios-dir {scenarios_dir} & ( cd cmd/observer-testbench/ui && npm install && npm run dev ) &"
    )


def log(log_file: str, message: str, prefix: str = "INFO"):
    main_prefix = "[q-logs]"

    with open(log_file, "a") as f:
        f.write(f"{main_prefix} [{prefix}] {message}\n")

    if os.environ.get("CC_VERBOSE", None) == "1":
        print(f"log: {main_prefix} [{prefix}] {message}")


def log_http(log_file: str, error: bool = False):
    if error:
        status = random.choice([400, 401, 403, 404, 500, 502, 503, 504])
        path = random.choice(["/api/v1/logs", "/api/v1/logs/search"])
        log(log_file, f"HTTP request {path} {status}", "ERROR")
    else:
        status = random.choice([200, 201])
        path = random.choice(["/api/v1/logs", "/api/v1/logs/search"])
        log(log_file, f"HTTP request {path} {status}")


@task
def log_incident(
    ctx,
    log_file: str = "/tmp/cc.log",
    baseline_s: int = 30,
    warmup_s: int = 10,
    incident_s: int = 10,
    recovery_s: int = 10,
    log_freq: int = 10,
    log_error_freq: int = 25,
):
    """
    Will simulate a log incident.

    Phase 1: Baseline
    - Regular logs
    Phase 2: Warm up
    - Few errors
    Phase 3: Incident
    - Many errors
    Phase 4: Recovery
    - Like baseline
    """

    start_time = time.time()
    print('Phase 1: Baseline')
    while time.time() - start_time < baseline_s:
        log_http(log_file)
        time.sleep(1 / log_freq)

    print('Phase 2: Warm up')
    start_time = time.time()
    while time.time() - start_time < warmup_s:
        log_http(log_file, error=random.random() < 2 / 10)
        time.sleep(1 / log_freq)

    print('Phase 3: Incident')
    start_time = time.time()
    while time.time() - start_time < incident_s:
        if random.random() < 1 / 10:
            log_http(log_file)
        log_http(log_file, error=True)
        time.sleep(1 / log_error_freq)

    print('Phase 4: Recovery')
    start_time = time.time()
    while time.time() - start_time < recovery_s:
        log_http(log_file)
        time.sleep(1 / log_freq)
