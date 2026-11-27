#!/usr/bin/env python3
"""Run the local AI code review for /create-pr with the first available reviewer.

Reviewers, in order:
  1. codex on the host, the same provider as the PR `Code review` workflow
  2. claude on the host
  3. claude in a `dda env dev` developer environment, which authenticates through `ddtool`

If a review fails, the next reviewer is tried. `--self claude|codex`, anywhere in the arguments,
names the AI agent running /create-pr: reviewers of that provider are skipped, so the change gets
a second opinion from another provider. Without `--self` (or with an empty value), the provider is
detected from the environment: `CLAUDECODE` for Claude Code, `CODEX_*` for Codex. When both are
set, e.g. Codex started from a Claude Code shell, the provider is unknown and no reviewer is
skipped. Those variables are inherited by child processes, so `--self none` turns detection off and
skips no reviewer, for agents other than claude and codex and for humans. Every other argument is passed to `dda review`.

The review goes to stdout; progress and the reasons a reviewer was skipped go to stderr.

Exit codes:
  0  a review completed
  1  every attempted review failed
  2  no reviewer is available
  3  the script can't run as invoked: an invalid `--self` value, or not in a git checkout

Stdlib only. Tests: `python3 test_review.py`.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path

PROVIDERS = ("claude", "codex")
NO_PROVIDER = "none"
EXIT_OK, EXIT_FAILED, EXIT_UNAVAILABLE, EXIT_USAGE = 0, 1, 2, 3


def log(message: str) -> None:
    print(f"create-pr review: {message}", file=sys.stderr, flush=True)


def parse_args(argv: Sequence[str]) -> tuple[str, list[str]]:
    """Split `--self VALUE` / `--self=VALUE` out of argv; return (self, arguments for dda review)."""
    self_provider = ""
    rest: list[str] = []
    args = iter(argv)
    for arg in args:
        if arg == "--self":
            self_provider = next(args, "")
        elif arg.startswith("--self="):
            self_provider = arg.removeprefix("--self=")
        else:
            rest.append(arg)
    return self_provider, rest


def detect_self(env: Mapping[str, str]) -> str:
    """Return the provider of the agent running this script, or "" when it can't tell."""
    in_claude = bool(env.get("CLAUDECODE"))
    in_codex = any(env.get(name) for name in ("CODEX_THREAD_ID", "CODEX_SANDBOX", "CODEX_SANDBOX_NETWORK_DISABLED"))
    if in_claude and in_codex:
        log("both Claude Code and Codex variables are set, pass --self to skip your own provider")
        return ""
    if in_claude:
        return "claude"
    if in_codex:
        return "codex"
    return ""


def succeeds(*cmd: str) -> bool:
    """Run a check command quietly and report whether it exited with 0."""
    return subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False).returncode == 0


@dataclass
class Reviewer:
    label: str
    provider: str
    # Returns the command that runs the review, or None after logging why this reviewer is unavailable
    prepare: Callable[[list[str]], list[str] | None]


def host_reviewer(provider: str, login_check: list[str], login_hint: str, label: str) -> Reviewer:
    def prepare(dda_args: list[str]) -> list[str] | None:
        if not shutil.which(provider):
            log(f"{provider} is not installed")
            return None
        if not succeeds(provider, *login_check):
            log(f"{provider} is installed but not logged in, run `{login_hint}`")
            return None
        provider_args = [] if provider == "codex" else ["--provider", provider]
        return ["dda", "review", *provider_args, *dda_args]

    return Reviewer(label, provider, prepare)


def dev_env_id(repo_root: str) -> str:
    """One environment per checkout, reused across runs since creating it pulls a large image.

    The suffix is the POSIX `cksum` of the checkout path, as earlier shell versions of this script
    computed it, so environments they created keep being reused.
    """
    out = subprocess.run(["cksum"], input=repo_root, capture_output=True, text=True, check=True).stdout
    return f"create-pr-review-{out.split()[0]}"


def dev_env_reviewer(repo_root: str) -> Reviewer:
    def discard(env_id: str) -> None:
        # Removed so the next run recreates it with newer dda and image versions
        log(f"removing developer environment {env_id}")
        if succeeds("dda", "env", "dev", "stop", "--id", env_id):
            succeeds("dda", "env", "dev", "remove", "--id", env_id)

    def prepare(dda_args: list[str]) -> list[str] | None:
        if Path("/.started").exists():
            log("already running in a developer environment")
            return None
        if not shutil.which("docker"):
            log("docker is not installed, which developer environments need")
            return None

        env_id = dev_env_id(repo_root)
        status = subprocess.run(
            ["dda", "env", "dev", "status", "--id", env_id], capture_output=True, text=True, check=False
        ).stdout
        state = next((line.removeprefix("State: ") for line in status.splitlines() if line.startswith("State: ")), "")
        if state != "started":
            log(
                f"starting developer environment {env_id} (state: {state or 'unknown'}), the first start pulls the image"
            )
            # Progress to stderr, so stdout only carries the review
            start = ["dda", "env", "dev", "start", "--id", env_id, "--repo", repo_root]
            if subprocess.run(start, stdout=sys.stderr, check=False).returncode != 0:
                return None

        in_env = ["dda", "env", "dev", "run", "--id", env_id, "--"]
        if not succeeds(*in_env, "git", "rev-parse", "--git-dir"):
            log(f"git does not work in {env_id}, which happens with Git worktrees and an older dda: update dda")
            discard(env_id)
            return None
        if not succeeds(*in_env, "npm", "--version"):
            log(
                f"npm is missing in {env_id}, so its image predates claude support: it is recreated with a newer one next run"
            )
            discard(env_id)
            return None
        if not succeeds(*in_env, "ddtool", "auth", "token", "rapid-ai-platform", "--datacenter", "us1.ddbuild.io"):
            log(f"claude in {env_id} has no credentials: `ddtool auth token rapid-ai-platform` failed")
            return None

        log(f"developer environment {env_id} is kept for later runs, remove it with `dda env dev remove --id {env_id}`")
        return [*in_env, "dda", "review", "--provider", "claude", *dda_args]

    return Reviewer("claude in a developer environment (CI uses codex, so findings may differ)", "claude", prepare)


def default_reviewers(repo_root: str) -> list[Reviewer]:
    return [
        host_reviewer("codex", ["login", "status"], "codex login", "codex (same reviewer as CI)"),
        host_reviewer(
            "claude", ["auth", "status"], "claude auth login", "claude (CI uses codex, so findings may differ)"
        ),
        dev_env_reviewer(repo_root),
    ]


def review(
    reviewers: Sequence[Reviewer],
    self_provider: str,
    dda_args: list[str],
    run: Callable[[list[str]], int] = lambda cmd: subprocess.run(cmd, check=False).returncode,
) -> int:
    """Try each reviewer not from `self_provider` in turn; return the exit code."""
    attempted = False
    for reviewer in reviewers:
        if reviewer.provider == self_provider:
            continue
        cmd = reviewer.prepare(dda_args)
        if cmd is None:
            continue
        attempted = True
        log(f"reviewing with {reviewer.label}")
        if run(cmd) == 0:
            return EXIT_OK
        log(f"the review with {reviewer.label} failed")

    if attempted:
        return EXIT_FAILED
    if self_provider:
        log(
            f"no reviewer from another provider than {self_provider} is available (reasons above): review the change with a subagent"
        )
    else:
        log("ALERT: no AI reviewer is available, so no local review ran (reasons above). To enable it, either:")
        log("  - install codex and run `codex login` (same reviewer as CI)")
        log("  - install Claude Code and log in")
        log("  - fix what prevents running claude in a `dda env dev` developer environment, which needs docker")
    return EXIT_UNAVAILABLE


def resolve_self(requested: str, env: Mapping[str, str]) -> str | None:
    """Return the provider whose reviewers to skip ("" for none), or None for an invalid value."""
    if not requested:
        return detect_self(env)
    if requested == NO_PROVIDER:
        return ""
    if requested in PROVIDERS:
        return requested
    log(f"unknown --self value '{requested}', expected claude, codex or none")
    return None


def main(argv: Sequence[str]) -> int:
    requested, dda_args = parse_args(argv)
    self_provider = resolve_self(requested, os.environ)
    if self_provider is None:
        return EXIT_USAGE
    if self_provider:
        log(f"running as {self_provider}, so {self_provider} reviewers are skipped")

    if not shutil.which("dda"):
        log("dda is not installed, see https://datadoghq.dev/datadog-agent-dev/")
        return EXIT_UNAVAILABLE
    toplevel = subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True, check=False)
    if toplevel.returncode != 0:
        log(f"not in a git checkout, run this from the repository: {toplevel.stderr.strip()}")
        return EXIT_USAGE
    repo_root = toplevel.stdout.strip()
    os.chdir(repo_root)

    return review(default_reviewers(repo_root), self_provider, dda_args)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
