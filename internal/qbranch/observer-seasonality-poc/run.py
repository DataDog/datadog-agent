#!/usr/bin/env python3
"""Run the Observer seasonality POC tests and resource profiles from this checkout."""

import argparse
import os
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[2]
PACKAGE = "comp/anomalydetection/observer/impl"


def run(args, cwd, env=None):
    subprocess.run([str(arg) for arg in args], cwd=cwd, env=env, check=True)


def environment():
    env = {
        key: value
        for key, value in os.environ.items()
        if not key.startswith(("OBSERVER_REAL_SEASONAL_", "OBSERVER_HOURLY_", "OBSERVER_RESEARCH_", "SEASONAL_"))
    }
    env.update(
        GOTOOLCHAIN=f"go{(REPO / '.go-version').read_text().strip()}",
        CGO_ENABLED="0",
        GOMAXPROCS="8",
        GOGC="100",
        GOMEMLIMIT="off",
        OBSERVER_REAL_SEASONAL_LEARNING_PLAN="3-2-1",
        OBSERVER_REAL_SEASONAL_CORRELATION="on",
        OBSERVER_REAL_SEASONAL_INTEGRATION="scratch_readers",
    )
    return env


def test(checkout, env, pattern):
    run(
        [
            "dda",
            "inv",
            "test-legacy",
            "--module=.",
            f"--targets=./{PACKAGE}",
            f"--test-run-name={pattern}",
            "--timeout=1800",
            "--extra-args=-tags=python,anomalydetectiontestbench,test -count=1 -v",
        ],
        checkout,
        env,
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["test", "build", "profile"])
    parser.add_argument("--fixture", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--arm", choices=["raw", "structure", "automatic"], default="automatic")
    parser.add_argument("--validate-emissions", action="store_true")
    args = parser.parse_args()
    checkout = REPO
    env = environment()
    if args.command == "test":
        # Older preservation tests intentionally exercise the original ten-hour schedule.
        test(
            checkout,
            dict(env, OBSERVER_REAL_SEASONAL_LEARNING_PLAN="6-2-2"),
            "^Test(BOCPD|RealSeasonal|Seasonal|EarlySeasonal)",
        )
        test(
            checkout, env, "^Test(EarlySeasonal|SeasonalIntegration|SeasonalDiscovery|RealSeasonalClosedSlotCausality)"
        )
    elif args.command == "build":
        run(["dda", "inv", "anomalydetection.build-testbench"], checkout, env)
    else:
        if args.fixture is None or args.output is None:
            parser.error("profile requires --fixture and --output")
        if args.output.exists():
            parser.error("profile output must be a new directory")
        env.update(
            SEASONAL_COST_INPUT=str(args.fixture.resolve()),
            SEASONAL_COST_OUTPUT=str(args.output.resolve()),
            SEASONAL_COST_N="1200",
            SEASONAL_COST_MODE=args.arm,
            SEASONAL_ATTRIBUTION_STRUCTURE_ONLY="1" if args.arm == "structure" else "0",
            SEASONAL_ATTRIBUTION_VALIDATE="1" if args.validate_emissions else "0",
        )
        test(checkout, env, "^TestSeasonalStageCost$")


if __name__ == "__main__":
    main()
