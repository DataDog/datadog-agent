#!/usr/bin/env python3
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# Copyright 2026-present Datadog, Inc.
"""Validate real USM HTTPS capture and shutdown in an isolated Linux container."""

import argparse
import json
import platform
import subprocess
import uuid
from pathlib import Path


def command(*args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=False)
    repo = Path(__file__).resolve().parents[4]
    targets = {
        "//pkg/network/usm:usm_test_bpf": "/usm-test",
        "//pkg/network/protocols/tls/gotls/testutil/gotls_client:gotls_client": "/gotls-client",
        "//pkg/network/ebpf/c/runtime:usm": "/bpf/co-re/usm.o",
        "//pkg/network/ebpf/c/runtime:usm-debug": "/bpf/co-re/usm-debug.o",
        "//pkg/network/ebpf/c/runtime:shared-libraries": "/bpf/co-re/shared-libraries.o",
        "//pkg/network/ebpf/c/runtime:shared-libraries-debug": "/bpf/co-re/shared-libraries-debug.o",
    }
    with (args.output / "build.log").open("w") as log:
        command("bazel", "build", "--strip=never", *targets, cwd=repo, stdout=log, stderr=subprocess.STDOUT)
    outputs = {}
    for target, destination in targets.items():
        result = command(
            "bazel", "cquery", "--strip=never", target, "--output=files", cwd=repo, capture_output=True, text=True
        )
        files = result.stdout.splitlines()
        if len(files) != 1:
            raise RuntimeError(f"expected one output for {target}: {files}")
        outputs[repo / files[0]] = destination

    container = "usm-multi-https-" + uuid.uuid4().hex[:12]
    metadata = {"host": platform.uname()._asdict(), "image": args.image}
    metadata["commit"] = command("git", "rev-parse", "HEAD", cwd=repo, capture_output=True, text=True).stdout.strip()
    metadata["image_id"] = command(
        "docker", "image", "inspect", "--format={{.Id}}", args.image, capture_output=True, text=True
    ).stdout.strip()
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
        command("docker", "exec", container, "mkdir", "-p", "/bpf/co-re")
        for source, destination in outputs.items():
            command("docker", "cp", str(source), container + ":" + destination)
        command("docker", "exec", container, "chown", "-R", "0:0", "/bpf")
        command("docker", "exec", container, "mount", "-t", "tracefs", "-o", "ro", "tracefs", "/sys/kernel/tracing")
        command("docker", "exec", container, "touch", "/tmp/empty-trace-events")
        for name in ("kprobe_events", "uprobe_events"):
            destination = "/sys/kernel/tracing/" + name
            command("docker", "exec", container, "mount", "--bind", "/tmp/empty-trace-events", destination)
            command("docker", "exec", container, "mount", "-o", "remount,bind,ro", destination)
        with (args.output / "https.log").open("w") as log:
            command(
                "docker",
                "exec",
                "-e",
                "DD_USM_UPROBE_MULTI_INTEGRATION=1",
                "-e",
                "DD_USM_UPROBE_MULTI_CLIENT=/gotls-client",
                "-e",
                "DD_SYSTEM_PROBE_BPF_DIR=/bpf",
                "-e",
                "DD_LOG_LEVEL=info",
                container,
                "/usm-test",
                "-test.run=^TestUSMUprobeMultiHTTPS$",
                "-test.v",
                "-test.timeout=5m",
                stdout=log,
                stderr=subprocess.STDOUT,
                timeout=360,
            )
    finally:
        subprocess.run(["docker", "rm", "-f", container], check=False, timeout=600, capture_output=True)
    print(f"HTTPS capture and shutdown checks passed: {args.output / 'https.log'}")


if __name__ == "__main__":
    main()
