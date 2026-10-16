#!/usr/bin/env python3
"""Prepare and run the pinned Observer seasonality POC in a separate checkout."""

import argparse
import hashlib
import json
import os
import shutil
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parent
MANIFEST = ROOT / "manifest.json"
PACKAGE = "comp/anomalydetection/observer/impl"


def run(args, cwd, env=None):
    subprocess.run([str(arg) for arg in args], cwd=cwd, env=env, check=True)


def verified_manifest():
    manifest = json.loads(MANIFEST.read_text())
    for source, item in manifest["files"].items():
        assert hashlib.sha256((ROOT / source).read_bytes()).hexdigest() == item["sha256"], source
        destination = Path(item["destination"])
        if destination.is_absolute() or ".." in destination.parts:
            raise ValueError(f"Invalid destination: {destination}")
    for source, digest in manifest["patches"].items():
        assert hashlib.sha256((ROOT / source).read_bytes()).hexdigest() == digest, source
    return manifest


def environment():
    env = {
        key: value
        for key, value in os.environ.items()
        if not key.startswith(("OBSERVER_REAL_SEASONAL_", "OBSERVER_HOURLY_", "OBSERVER_RESEARCH_", "SEASONAL_"))
    }
    env.update(
        GOTOOLCHAIN="go1.26.7",
        CGO_ENABLED="0",
        GOMAXPROCS="8",
        GOGC="100",
        GOMEMLIMIT="off",
        OBSERVER_REAL_SEASONAL_LEARNING_PLAN="3-2-1",
        OBSERVER_REAL_SEASONAL_CORRELATION="on",
        OBSERVER_REAL_SEASONAL_INTEGRATION="scratch_readers",
    )
    return env


def prepare(repo, checkout, manifest):
    if checkout.exists():
        raise ValueError("Choose a new checkout path; existing directories are never overwritten")
    run(["git", "worktree", "add", "--detach", checkout, manifest["base"]], repo)
    patches = [ROOT / name for name in manifest["patches"]]
    run(["git", "apply", "--check", *patches], checkout)
    run(["git", "apply", *patches], checkout)
    for name, item in manifest["files"].items():
        dest = checkout / item["destination"]
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(ROOT / name, dest)
    (checkout / ".seasonality-poc.json").write_text(
        json.dumps({"base": manifest["base"], "manifest_sha256": hashlib.sha256(MANIFEST.read_bytes()).hexdigest()})
        + "\n"
    )


def check_checkout(checkout, manifest):
    marker = json.loads((checkout / ".seasonality-poc.json").read_text())
    head = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=checkout, text=True).strip()
    if head != manifest["base"] or marker["manifest_sha256"] != hashlib.sha256(MANIFEST.read_bytes()).hexdigest():
        raise ValueError("Checkout does not match this POC; prepare a new checkout")


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
    parser.add_argument("command", choices=["prepare", "test", "build", "profile"])
    parser.add_argument("--checkout", type=Path, required=True)
    parser.add_argument("--repo", type=Path, default=ROOT.parents[2])
    parser.add_argument("--fixture", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--arm", choices=["raw", "structure", "automatic"], default="automatic")
    parser.add_argument("--validate-emissions", action="store_true")
    args = parser.parse_args()
    manifest = verified_manifest()
    checkout = args.checkout.resolve()
    if args.command == "prepare":
        prepare(args.repo.resolve(), checkout, manifest)
        return
    check_checkout(checkout, manifest)
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
