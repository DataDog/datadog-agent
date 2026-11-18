"""Measure the complete hook in an isolated, full repository snapshot.

Supply a `git archive` tar containing the proposed configuration and runner.
Tool setup must be complete on PATH. Buildifier and the managed Python hook
environment are installed before timing. Ruff's cache is disabled by the
runner. No build or lint-result cache is needed by these validators.
"""

import argparse
import hashlib
import json
import math
import os
import platform
import re
import subprocess
import sys
import tarfile
import tempfile
import time
from pathlib import Path

SOURCE_REPOSITORY = Path(__file__).resolve().parents[1]
EXPECTED_HOOKS = {
    "protected-branches",
    "go-fmt",
    "shellcheck",
    "actionlint",
    "copyright",
    "shell-check-no-set-x",
    "python-ruff",
    "buildifier",
    "rustfmt",
}


def fixture_environment(root):
    env = {
        key: value for key, value in os.environ.items() if key != "SKIP" and not key.startswith(("GIT_", "PRE_COMMIT_"))
    }
    env.update(
        GIT_CONFIG_GLOBAL=os.devnull,
        GIT_CONFIG_NOSYSTEM="1",
        GIT_CONFIG_COUNT="1",
        GIT_CONFIG_KEY_0="core.attributesFile",
        GIT_CONFIG_VALUE_0=os.devnull,
        GIT_ATTR_NOSYSTEM="1",
        GIT_NO_REPLACE_OBJECTS="1",
        PRE_COMMIT_HOME=str(root / ".cache" / "qualification" / "tools"),
        PRE_COMMIT_COLOR="never",
        SHELLCHECK_OPTS="--format=gcc",
    )
    return env


def archive_payload(snapshot, member):
    if member.issym():
        return member.linkname.encode("utf-8", "surrogateescape")
    with snapshot.extractfile(member) as source:
        return source.read()


def blob_digest(payload):
    digest = hashlib.sha1(f"blob {len(payload)}\0".encode(), usedforsecurity=False)
    digest.update(payload)
    return digest.digest()


def verified_archive(snapshot, source_repository, env):
    revision = snapshot.pax_headers.get("comment", "").strip()
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("Create the qualification archive with git archive from a commit.")
    kind = subprocess.check_output(["git", "cat-file", "-t", revision], cwd=source_repository, env=env)
    if kind.strip() != b"commit":
        raise ValueError("The qualification archive must identify a commit.")
    tree = subprocess.check_output(["git", "ls-tree", "-rz", "--full-tree", revision], cwd=source_repository, env=env)
    expected = {}
    for record in tree.split(b"\0")[:-1]:
        metadata, path = record.split(b"\t", 1)
        mode, kind, _ = metadata.decode("ascii").split()
        if kind == "blob":
            expected[path.decode("utf-8", "surrogateescape")] = mode
    members = {}
    digests = {}
    for member in snapshot.getmembers():
        if member.isdir():
            continue
        if not (member.isfile() or member.issym()) or member.name not in expected:
            raise ValueError(f"Unexpected entry in qualification archive: {member.name}")
        mode = "120000" if member.issym() else "100755" if member.mode & 0o111 else "100644"
        if expected.pop(member.name) != mode:
            raise ValueError(f"Archive mode differs from {revision}: {member.name}")
        digests[member.name] = blob_digest(archive_payload(snapshot, member))
        members[member.name] = (mode, member)
    if expected:
        raise ValueError(f"Qualification archive omits {len(expected)} tracked files, including {next(iter(expected))}")
    # git archive applies committed attributes, including eol=crlf. Compare
    # archived payloads to Git's own rendering of the verified full tree.
    with (
        tempfile.TemporaryDirectory(prefix="agent-hook-source-") as clean_source,
        tempfile.TemporaryFile() as canonical,
    ):
        # Borrow only the objects; local info/attributes must not change identity.
        subprocess.run(
            ["git", "clone", "--quiet", "--bare", "--shared", "--template=", str(source_repository), clean_source],
            env=env,
            check=True,
        )
        subprocess.run(
            ["git", "archive", "--format=tar", revision], cwd=clean_source, env=env, stdout=canonical, check=True
        )
        canonical.seek(0)
        with tarfile.open(fileobj=canonical, mode="r|") as reference:
            for member in reference:
                if member.isdir():
                    continue
                if digests.pop(member.name, None) != blob_digest(archive_payload(reference, member)):
                    raise ValueError(f"Archive content differs from {revision}: {member.name}")
    if digests:
        raise ValueError(f"Git did not archive the full source tree, including {next(iter(digests))}")
    return revision, members


def classify(name, code, output):
    lines = output.splitlines()
    statuses = []
    for line in lines:
        if match := re.fullmatch(r"([\w-]+)\.{2,}(Passed|Failed|Skipped)", line):
            statuses.append(match.groups())
    if len(statuses) != len(EXPECTED_HOOKS) or {hook for hook, _ in statuses} != EXPECTED_HOOKS:
        return "unexpected_failure"
    failed = {hook: status for hook, status in statuses if status != "Passed"}
    if code == 0 and not failed:
        return "passed"
    if name == "shellcheck-settings" and code == 1 and failed == {"shellcheck": "Failed"}:
        # The GCC output format emits exactly one location and SC code per finding.
        details = [line for line in lines if line and not re.fullmatch(r"[\w-]+\.{2,}(Passed|Failed)", line)]
        if (
            details[:2] == ["- hook id: shellcheck", "- exit code: 1"]
            and details[2:]
            and all(re.fullmatch(r".+:\d+:\d+: (?:error|warning|note): .+ \[SC\d{4}\]", line) for line in details[2:])
        ):
            return "shellcheck_findings"
    return "unexpected_failure"


def benchmark(archive, runs, source_repository=SOURCE_REPOSITORY, report_path=None):
    with (
        tempfile.TemporaryDirectory(prefix="agent-hook-qualification-") as temporary,
        tarfile.open(archive) as snapshot,
    ):
        root = Path(temporary)
        env = fixture_environment(root)
        revision, members = verified_archive(snapshot, source_repository, env)

        def git(*args):
            return subprocess.check_output(["git", *args], cwd=root, env=env, stderr=subprocess.STDOUT)

        def write(path, content):
            target = root / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(content, encoding="utf-8", newline="\n")

        git("init", "--initial-branch=hook-qualification", "--template=")
        git("config", "user.name", "Hook qualification")
        git("config", "user.email", "hooks@example.com")
        git("config", "core.autocrlf", "false")
        git("config", "core.symlinks", "false" if os.name == "nt" else "true")
        git("config", "core.longpaths", "true")
        if os.name == "nt":
            # Retain Git for Windows' default process-local metadata cache
            # while excluding unrelated global and system settings.
            git("config", "core.fscache", "true")
        git("config", "maintenance.auto", "false")
        git("config", "gc.auto", "0")
        header = (
            "// Unless explicitly stated otherwise all files in this repository are licensed\n"
            "// under the Apache License Version 2.0.\n"
            "// This product includes software developed at Datadog (https://www.datadoghq.com/).\n"
            "// Copyright 2026-present Datadog, Inc.\n\n"
        )
        samples = {
            "hook_qualification.go": header + "package hooksample\n\nvar value = 1\n",
            "hook_qualification.py": "value = 1\nother = 2\n",
            "hook_qualification.rs": "pub fn value() -> i32 {\n    1\n}\n",
            "hook_qualification.bzl": '"""Hook qualification."""\n\nVALUE = 1\n',
            "hook_qualification.sh": "#!/bin/bash\nprintf '%s\\n' ready\n",
            ".github/workflows/hook-qualification.yml": (
                "name: Hook qualification\non: workflow_dispatch\njobs:\n  qualify:\n"
                "    runs-on: ubuntu-latest\n    steps:\n      - run: echo ready\n"
            ),
        }
        if collision := samples.keys() & members.keys():
            raise ValueError(f"Qualification samples already exist in the source: {sorted(collision)}")
        # Import a pack instead of creating tens of thousands of loose objects
        # on Windows. Include every archived file, even if now gitignored.
        with subprocess.Popen(
            ["git", "fast-import", "--quiet"],
            cwd=root,
            env=env,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
        ) as importer:
            assert importer.stdin is not None
            assert importer.stdout is not None

            def data(payload):
                importer.stdin.write(f"data {len(payload)}\n".encode() + payload + b"\n")

            importer.stdin.write(
                f"commit refs/heads/hook-qualification\n"
                f"committer Hook qualification <hooks@example.com> {int(time.time())} +0000\n".encode()
            )
            data(b"Qualification snapshot\n")
            for path, (mode, member) in members.items():
                importer.stdin.write(f"M {mode} inline {json.dumps(path, ensure_ascii=False)}\n".encode())
                data(archive_payload(snapshot, member))
            for path, content in samples.items():
                importer.stdin.write(f"M 100644 inline {path}\n".encode())
                data(content.encode())
            importer.stdin.write(b"\n")
            importer.stdin.close()
            output = importer.stdout.read()
            if importer.wait():
                raise RuntimeError(output.decode("utf-8", errors="replace"))
        git("reset", "--hard", "HEAD")
        # Archive blobs can contain CRLF that checkout attributes normalize.
        # Establish a clean baseline rather than timing repeated stashes of it.
        changed = git("diff", "--name-only", "-z").decode().split("\0")[:-1]
        for offset in range(0, len(changed), 64):
            git("add", "--renormalize", "--", *changed[offset : offset + 64])
        if changed:
            git("commit", "--amend", "--no-edit")
        if status := git("status", "--porcelain", "--untracked-files=no"):
            raise RuntimeError(f"The qualification snapshot is not clean:\n{status.decode()}")
        subprocess.run([sys.executable, "tasks/hooks.py", "setup-buildifier"], cwd=root, env=env, check=True)
        subprocess.run(["pre-commit", "install", "--install-hooks"], cwd=root, env=env, check=True)

        def state(paths):
            return (
                git("write-tree"),
                git("diff", "--binary"),
                git("ls-files", "--others", "--exclude-standard", "-z"),
                {path: (root / path).read_bytes() for path in paths},
            )

        def invoke(name, command, paths, baseline, timed):
            start = time.perf_counter()
            try:
                completed = subprocess.run(command, cwd=root, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
                code = completed.returncode
                output = completed.stdout.decode("utf-8", errors="replace")
            except OSError as error:
                code = None
                output = str(error)
            seconds = time.perf_counter() - start
            record = {"exit_code": code, "output": output, "classification": classify(name, code, output)}
            if timed:
                record["seconds"] = seconds
            try:
                record["preserved_index_and_worktree"] = state(paths) == baseline
            except (OSError, subprocess.CalledProcessError) as error:
                record["preserved_index_and_worktree"] = False
                record["preservation_error"] = str(error)
            return record

        preflight = invoke("preflight", ["pre-commit", "run", "--files", *samples], samples, state(samples), False)
        report = {
            "source_revision": revision,
            "platform": platform.platform(),
            "python": sys.version,
            "preflight": preflight,
            "scenarios": {},
            "completed": False,
            "phase": "preflight",
        }

        def checkpoint():
            if report_path is not None:
                report_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")

        checkpoint()
        if preflight["classification"] != "passed" or not preflight["preserved_index_and_worktree"]:
            return report
        scenarios = {
            "documentation": ["doc/setup/optional.md"],
            "mixed": list(samples),
            "mixed-crlf": list(samples),
            "partially-staged": ["hook_qualification.py"],
            "go-settings": [".go-version"],
            "python-settings": ["pyproject.toml"],
            "buildifier-settings": [".buildifier.json"],
            "rust-settings": ["Cargo.toml"],
            "shellcheck-settings": [".shellcheckrc"],
            "actionlint-settings": [".github/actionlint.yaml"],
            "copyright-rules": ["tasks/libs/types/copyright.py"],
        }
        for name, paths in scenarios.items():
            report["phase"] = f"{name}:prepare"
            checkpoint()
            git("reset", "--hard", "HEAD")
            if name == "shellcheck-settings":
                write(".shellcheckrc", "# Qualification.\n")
            else:
                for path in paths:
                    if name in {"mixed", "mixed-crlf"}:
                        write(
                            path,
                            (root / path)
                            .read_text("utf-8")
                            .replace("= 1", "= 2")
                            .replace("    1\n", "    2\n")
                            .replace("ready", "checkpoint"),
                        )
                        if name == "mixed-crlf":
                            target = root / path
                            target.write_bytes(target.read_bytes().replace(b"\n", b"\r\n", 1))
                        continue
                    suffix = "// Qualification.\n" if path.endswith((".go", ".rs")) else "# Qualification.\n"
                    if path in {".go-version", ".buildifier.json"}:
                        suffix = "\n"
                    write(path, (root / path).read_text("utf-8") + suffix)
            git("add", "--", *paths)
            if name == "partially-staged":
                write("hook_qualification.py", "value=  2\n")
            baseline = state(paths)
            invocations = []
            scenario = {"invocations": invocations}
            report["scenarios"][name] = scenario
            report["phase"] = name
            for _ in range(runs):
                record = invoke(name, ["git", "hook", "run", "pre-commit"], paths, baseline, True)
                invocations.append(record)
                checkpoint()
                if record["classification"] == "unexpected_failure" or not record["preserved_index_and_worktree"]:
                    return report
            durations = [entry["seconds"] for entry in invocations]
            scenario.update(
                runs=runs,
                first_seconds=round(durations[0], 3),
                p95_seconds=round(sorted(durations)[math.ceil(0.95 * runs) - 1], 3),
                max_seconds=round(max(durations), 3),
                exit_codes=sorted({entry["exit_code"] for entry in invocations}),
                preserved_index_and_worktree=True,
            )
            print(name, json.dumps({k: v for k, v in scenario.items() if k != "invocations"}), flush=True)
        report["completed"] = True
        report["phase"] = "complete"
        checkpoint()
        return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--source-repository", type=Path, default=SOURCE_REPOSITORY)
    parser.add_argument("--runs", type=int, default=20)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if args.runs < 1:
        parser.error("--runs must be positive")
    report = benchmark(args.archive.resolve(), args.runs, args.source_repository.resolve(), args.output.resolve())
    if not report["completed"]:
        sys.exit(f"Qualification stopped during {report['phase']}. See {args.output}.")
