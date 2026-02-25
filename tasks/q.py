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
