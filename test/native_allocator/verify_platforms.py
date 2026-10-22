"""Check target-platform dependencies and final-link actions."""

import argparse
import json
import subprocess
import sys

ALLOCATOR = "jemalloc//|^//bazel/rules/native_allocator:compat"


def query(command, expression, *flags):
    return subprocess.check_output(["bazel", command, expression, *flags], text=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--platform", action="append", required=True)
    args = parser.parse_args()
    targets = "//cmd/agent + //cmd/trace-agent + //cmd/system-probe"
    if sys.platform == "linux":
        # Positive control: a label-format change must not turn every negative
        # check into a vacuous pass. Match apparent and canonical repo names.
        assert query("cquery", f"filter('{ALLOCATOR}', deps(//cmd/trace-agent))").strip()
    for platform in args.platform:
        flags = [f"--platforms=//bazel/platforms:{platform}"]
        deps = query("cquery", f"filter('{ALLOCATOR}', deps({targets}))", *flags)
        assert not deps.strip(), deps
        actions = json.loads(query("aquery", f'mnemonic("GoLink", {targets})', *flags, "--output=jsonproto"))
        assert actions.get("actions"), "No final-link actions inspected"
        for action in actions["actions"]:
            command = " ".join(action.get("arguments", []))
            for unexpected in ("jemalloc", "datadog-agent.map", "--export-dynamic", "-u,malloc"):
                assert unexpected not in command, command
        print(f"PASS {platform}: no allocator dependencies or export flags")

    deps = query(
        "cquery",
        f"filter('{ALLOCATOR}', deps(//cmd/trace-agent))",
        "--platforms=//bazel/platforms:linux_arm64",
        "--@rules_go//go/config:pure",
    )
    assert not deps.strip(), deps
    print("PASS Linux pure-Go: no allocator dependencies")


if __name__ == "__main__":
    main()
