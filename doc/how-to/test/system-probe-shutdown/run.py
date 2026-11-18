#!/usr/bin/env python3
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# Copyright 2026-present Datadog, Inc.
"""Run isolated, sequential uprobe exit scenarios and retain raw phase/stack data."""

import argparse
import hashlib
import json
import platform
import subprocess
import uuid
from pathlib import Path


def command(*args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True, help="Trusted Linux image with matching architecture and glibc")
    parser.add_argument("--output", type=Path, required=True, help="New output directory")
    parser.add_argument("--counts", default="128,512")
    parser.add_argument("--repetitions", type=int, default=1)
    parser.add_argument("--micro", action="store_true", help="Also compare 1/12 workers and multi groups of 1/32/all")
    args = parser.parse_args()
    counts = [int(value) for value in args.counts.split(",")]
    if not counts or any(value < 1 or value > 10000 for value in counts) or args.repetitions < 1:
        parser.error("counts must be between 1 and 10000 and repetitions must be positive")
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=False)
    repo = Path(__file__).resolve().parents[4]
    source = Path(__file__).with_name("probe_exit.c")
    executable = args.output / "probe-exit"
    command("cc", "-O2", "-pthread", "-Wall", "-Wextra", "-Werror", "-o", str(executable), str(source))

    targets = ["//pkg/ebpf/uprobes:attacher_test_bpf", "//pkg/ebpf/testdata/c:uprobe_attacher-test"]
    with (args.output / "build.log").open("w") as log:
        command("bazel", "build", *targets, cwd=repo, stdout=log, stderr=subprocess.STDOUT)
    outputs = []
    for target in targets:
        result = command("bazel", "cquery", target, "--output=files", cwd=repo, capture_output=True, text=True)
        files = result.stdout.splitlines()
        if len(files) != 1:
            raise RuntimeError(f"expected one output for {target}, got {files}")
        outputs.append(repo / files[0])

    container = "uprobe-shutdown-" + uuid.uuid4().hex[:12]
    metadata = {"host": platform.uname()._asdict(), "image": args.image, "counts": counts}
    metadata["commit"] = command("git", "rev-parse", "HEAD", cwd=repo, capture_output=True, text=True).stdout.strip()
    metadata["image_id"] = command(
        "docker", "image", "inspect", "--format={{.Id}}", args.image, capture_output=True, text=True
    ).stdout.strip()
    metadata["source_sha256"] = {
        str(path.relative_to(repo)): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in (
            source,
            Path(__file__).resolve(),
            *(
                repo / "pkg/ebpf/uprobes" / name
                for name in ("shutdown_test.go", "attacher.go", "multi.go", "multi_programs.go")
            ),
        )
    }
    (args.output / "metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")
    try:
        command(
            "docker",
            "run",
            "-d",
            "--name",
            container,
            "--network=none",
            "--privileged",
            "--entrypoint=/bin/sleep",
            args.image,
            "infinity",
            capture_output=True,
        )
        for src, dest in [(executable, "/probe-exit"), (outputs[0], "/attacher-test"), (outputs[1], "/uprobe-test.o")]:
            command("docker", "cp", str(src), container + ":" + dest)
        for repeat in range(args.repetitions):
            for count in counts:
                cases = [("agent", mode, sig) for mode in ("perf", "multi") for sig in ("term", "kill")]
                if args.micro:
                    cases += [
                        ("micro", mode, sig)
                        for mode in ("1", "12", "multi:1", "multi:32", f"multi:{count}")
                        for sig in ("term", "kill")
                    ]
                for backend, mode, sig in cases:
                    name = f"{repeat}-{backend}-{count}-{mode.replace(':', '-')}-{sig}"
                    cmd = ["docker", "exec"]
                    if backend == "agent":
                        env = {
                            "DD_UPROBE_SHUTDOWN_EXEC": "/attacher-test",
                            "DD_UPROBE_SHUTDOWN_MODE": mode,
                            "DD_UPROBE_SHUTDOWN_COUNT": str(count),
                            "DD_UPROBE_SHUTDOWN_TARGET": "/probe-exit",
                            "DD_UPROBE_SHUTDOWN_OBJECT": "/uprobe-test.o",
                        }
                        for key, value in env.items():
                            cmd += ["-e", key + "=" + value]
                    cmd += [container, "/probe-exit", "--supervise", str(count), mode, sig]
                    with (
                        (args.output / (name + ".stdout")).open("w") as out,
                        (args.output / (name + ".stacks")).open("w") as err,
                    ):
                        subprocess.run(cmd, check=True, stdout=out, stderr=err, timeout=max(180, count * 0.3 + 60))
                    records = []
                    for line in (args.output / (name + ".stdout")).read_text().splitlines():
                        if line.startswith("{"):
                            records.append(json.loads(line))
                    if not records or records[-1].get("phase") != "reaped":
                        raise RuntimeError(f"missing final measurement: {name}")
                    result = {"case": name, "records": records}
                    with (args.output / "results.jsonl").open("a") as output:
                        output.write(json.dumps(result) + "\n")
                    print(json.dumps(result), flush=True)
    finally:
        # Only remove the uniquely named container created by this invocation.
        subprocess.run(["docker", "rm", "-f", container], check=False, timeout=600, capture_output=True)


if __name__ == "__main__":
    main()
