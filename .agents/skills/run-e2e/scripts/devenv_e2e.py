#!/usr/bin/env python3
"""Prepare a `dda env dev` Linux container for running new-e2e tests, or tear one down.

A fresh dev env cannot run `dda inv -- new-e2e-tests.run` as-is: the container has no
E2E config, and Pulumi's plugins and backend selection live under `$HOME`, which is
not a persistent volume. `up` closes both gaps and is idempotent, so a reused
environment pays the cost once. It prints the exact command prefix to run tests with,
because the container needs `E2E_*` overrides that only this script knows.

Run `up` from a datadog-agent clone on the host. See /.agents/skills/run-e2e/references/devenv.md.
"""

from __future__ import annotations

import argparse
import copy
import functools
import getpass
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

try:
    import yaml
except ImportError:
    # Only `up` reads the config, so keep `down` — the path that stops stacks being orphaned —
    # working without PyYAML, and report the missing dependency from `up` instead.
    yaml = None

# `dda env dev start` derives the bind mount from the current directory's parent, so the
# checkout has to be a directory of this name for the repo to appear in the container.
REPO_NAME = "datadog-agent"
CONFIG_NAME = ".test_infra_config.yaml"
CONTAINER_HOME = "/home/dd"
# Deliberately outside $HOME: the image entrypoint runs `chown -R dd: /home/dd` on first
# start, which fails on a read-only mount and aborts the whole container startup.
CONTAINER_KEY_DIR = "/.e2e"
CONTAINER_REPO = f"/repos/{REPO_NAME}"
CONTAINER_CONFIG = f"{CONTAINER_HOME}/{CONFIG_NAME}"
ENV_TYPE = "linux-container"
SSO_PROFILE = "sso-agent-sandbox-account-admin-8h"
# `dda inv -- e2e.setup` writes `[profile exec-<sso profile>]` with a credential_process, and that
# derived profile is the one the framework authenticates as. Deriving it here keeps a role rename
# to a single edit. Not to be confused with the AWS_PROFILE environment variable, whose presence
# is itself a common cause of failures — see /.agents/skills/run-e2e/references/troubleshooting.md.
EXEC_PROFILE = f"exec-{SSO_PROFILE}"
PULUMI_PROJECT = "e2elocal"
SCRIPT = Path(__file__).name

# Key files the container needs, as (config section, field, required). The local provisioners read
# `configParams.local.publicKeyPath` unconditionally, so a local target needs it in the container
# even though it needs no cloud credentials at all. Azure and GCP would slot in the same way.
KEY_FIELDS = (
    ("aws", "privateKeyPath", True),
    ("aws", "publicKeyPath", True),
    ("local", "publicKeyPath", False),
)

# Exit codes. The skill branches on these, so each one maps to a distinct remedy.
ERROR = 1
NO_HOST_CONFIG = 2
UNUSABLE_CHECKOUT = 3
NO_AWS_SESSION = 4
ALREADY_IN_DEVENV = 5
STACK_STILL_LIVE = 6


def fail(code: int, message: str) -> None:
    print(message, file=sys.stderr)
    sys.exit(code)


def child_env() -> dict[str, str]:
    """Environment for `dda` subprocesses, scrubbed of things that break it."""
    env = dict(os.environ)
    # `dda` renders emoji; on a Windows console that raises UnicodeEncodeError unless
    # Python is forced to UTF-8.
    env["PYTHONUTF8"] = "1"
    # A parent `uv run` exports these, and they make the nested `uv` that bootstraps
    # `dda` resolve against the wrong environment.
    for var in [k for k in env if k.startswith("UV_")] + ["VIRTUAL_ENV"]:
        env.pop(var, None)
    return env


def run(
    command: list[str], *, cwd: Path | None = None, capture: bool = True, timeout: int | None = None
) -> subprocess.CompletedProcess:
    # When a child streams, its stdout goes to *our* stderr: `up --json` promises a machine-readable
    # document, and `dda env dev start` and `e2e.setup` are chatty enough to bury the JSON otherwise.
    streams = {"capture_output": True} if capture else {"stdout": sys.stderr}
    try:
        return subprocess.run(
            command,
            cwd=cwd,
            env=child_env(),
            text=True,
            encoding="utf-8",
            errors="replace",
            check=False,
            timeout=timeout,
            **streams,
        )
    except subprocess.TimeoutExpired:
        return subprocess.CompletedProcess(command, returncode=1, stdout="", stderr=f"timed out after {timeout}s")


@functools.cache
def dda() -> str:
    found = shutil.which("dda")
    if found is None:
        fail(ERROR, "`dda` is not on PATH. See /docs/public/setup/required.md.")
    return found


def env_command(instance: str, subcommand: str, *args: str, executable: str | None = None) -> list[str]:
    # `-t` is required on Windows hosts, where the default type is the unimplemented
    # `windows-container`, and is a no-op elsewhere.
    return [executable or dda(), "env", "dev", subcommand, "-t", ENV_TYPE, "--id", instance, *args]


def in_devenv() -> bool:
    """Whether this process is already inside a Linux dev env.

    The dev env entrypoint creates /.started once it has finished setting the container up.
    """
    return Path("/.started").is_file()


def resolve_repo_root() -> Path:
    result = run(["git", "rev-parse", "--show-toplevel"])
    if result.returncode != 0:
        fail(UNUSABLE_CHECKOUT, "Not inside a git repository. Run this from a datadog-agent clone.")
    root = Path(result.stdout.strip()).resolve()

    # The mount is `<parent>/datadog-agent`, and git just resolved this directory, so matching
    # the name is the whole check — the parent necessarily contains it.
    if root.name != REPO_NAME:
        fail(
            UNUSABLE_CHECKOUT,
            f"This checkout is at {root}, but the dev env mounts <parent>/{REPO_NAME}, so the\n"
            f"checkout directory must be named {REPO_NAME}. Either run from a clone that is, or\n"
            "run the tests on the host with --host.",
        )

    # In a worktree, .git is a file pointing into the main clone, which is outside every
    # mount. new-e2e-tests.run reads the commit SHA unconditionally, so it would abort.
    if not (root / ".git").is_dir():
        fail(
            UNUSABLE_CHECKOUT,
            f"{root} is a git worktree. Its .git points outside the container's mounts, so git\n"
            "commands fail there and the test run aborts before its preflight. Either run from\n"
            "the main clone, or run the tests on the host with --host.",
        )
    return root


def load_host_config() -> tuple[dict, dict[tuple[str, str], Path]]:
    """Return the host's E2E config and every key file it points at that has to reach the container.

    This is a pre-check, not the authority: `_check_e2e_local_config_or_exit` in
    /tasks/new_e2e_tests.py is what actually gates the run. Checking here only buys a failure
    before a container is started rather than 10 minutes into one, so if that function grows a
    new requirement, expect the run to fail on it despite `up` succeeding.

    Optional entries are skipped when absent rather than failing, so a machine set up only for AWS
    still works. Add a provider here and the mount, the path rewrite and the validation all follow.
    """
    config_path = Path.home() / CONFIG_NAME
    setup_hint = "Run `dda inv -- e2e.setup` on the host (~30s, one question, opens an SSO browser flow)."
    if not config_path.is_file():
        fail(NO_HOST_CONFIG, f"{config_path} does not exist. {setup_hint}")

    config = yaml.safe_load(config_path.read_text(encoding="utf-8")) or {}
    params = config.get("configParams") or {}
    if not (params.get("aws") or {}).get("keyPairName"):
        fail(NO_HOST_CONFIG, f"{config_path} has no configParams.aws.keyPairName. {setup_hint}")

    keys: dict[tuple[str, str], Path] = {}
    for section, field, required in KEY_FIELDS:
        value = (params.get(section) or {}).get(field)
        if not value:
            if required:
                fail(NO_HOST_CONFIG, f"{config_path} has no configParams.{section}.{field}. {setup_hint}")
            continue
        key_path = Path(value).expanduser()
        if not key_path.is_file():
            message = f"configParams.{section}.{field} points at {key_path}, which does not exist."
            if required:
                fail(NO_HOST_CONFIG, f"{message} {setup_hint}")
            print(f"{message} Targets that need it will fail.", file=sys.stderr)
            continue
        keys[section, field] = key_path

    return config, keys


def container_config_dir(instance: str) -> Path:
    return Path(tempfile.gettempdir()) / f"dda-e2e-{instance}"


def restrict_to_owner(path: Path) -> None:
    """Restrict a file holding secrets so that only its owner can read it.

    Condensed from `restrict_file_to_owner` in /tasks/e2e_framework/tool.py, which cannot be
    imported here because that module pulls in invoke. Windows is the common host for this
    path and there `os.chmod` only toggles the read-only attribute, so the ACL has to be
    replaced outright or the mode below does nothing at all.
    """
    if os.name != "nt":
        path.chmod(0o600)
        return

    user = os.environ.get("USERNAME") or getpass.getuser()
    # SYSTEM and Administrators are named by SID because their names are localized.
    result = run(
        [
            "icacls",
            str(path),
            "/inheritance:r",
            "/grant:r",
            f"{user}:(F)",
            "*S-1-5-18:(F)",
            "*S-1-5-32-544:(F)",
        ]
    )
    if result.returncode != 0:
        fail(
            ERROR,
            f"Could not restrict {path} to your account, so it would keep the ACLs it inherited —\n"
            "and it is about to hold the Pulumi passphrase and the Datadog API and app keys.\n"
            f"icacls said:\n{result.stderr.strip() or result.stdout.strip()}",
        )


def container_key(section: str, path: Path) -> str:
    """Where `up` mounts a host keyfile inside the container.

    Namespaced by config section so that two providers whose key files happen to share a basename
    cannot collide on one mount destination.
    """
    return f"{CONTAINER_KEY_DIR}/{section}-{path.name}"


def write_container_config(config: dict, instance: str, keys: dict[tuple[str, str], Path]) -> Path:
    """Write a copy of the E2E config whose key paths are the container's, and return its path.

    Rewriting the paths here rather than exporting `E2E_AWS_*_KEY_PATH` at run time keeps absolute
    container paths off the test command line. On a Windows host those get rewritten by MSYS path
    conversion when an agent runs the command through Git Bash — `/.e2e/key.pem` silently becomes
    `C:/Users/.../git/.e2e/key.pem` — and the run then fails when it tries to reach the VM.
    """
    config = copy.deepcopy(config)
    for (section, field), host_path in keys.items():
        config["configParams"][section][field] = container_key(section, host_path)

    # A second copy of the Pulumi passphrase and API keys, so keep the directory and the file to
    # this user. `/tmp` is world-readable on Linux.
    directory = container_config_dir(instance)
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    path = directory / CONFIG_NAME
    # Lock the file down while it is still empty, so the secrets are never briefly readable.
    path.touch()
    restrict_to_owner(path)
    path.write_text(yaml.safe_dump(config, sort_keys=False), encoding="utf-8")
    return path


def stack_suffix(extra: str | None) -> str:
    """A per-developer stack suffix.

    Stack names are prefixed with the OS username, which is `dd` for every developer's
    container, so without this two people running the same suite collide on stack and
    resource names in the shared cloud account. Sanitized the same way as
    `get_stack_name_prefix` in /tasks/e2e_framework/tool.py, so the suffix matches the naming
    the rest of the framework produces: EKS rejects `.`, and spaces cause trouble on Windows.
    """
    try:
        host_user = getpass.getuser()
    except Exception:  # noqa: BLE001 - no password entry and no matching env var
        host_user = "nouser"
    host_user = host_user.replace(".", "-").replace(" ", "-")
    return f"{host_user}-{extra}" if extra else host_user


def check_clone_repos_disabled() -> None:
    """Refuse to create an env that would clone the repo instead of mounting this tree.

    Cloning makes `dda env dev start` fetch datadog-agent from GitHub rather than bind-mounting
    the local checkout, so the tests would not exercise the working tree at all. There is no CLI
    flag to turn it off for a single command, hence the instruction rather than an override.

    Two settings turn it on — `env.dev.clone-repos`, and `clone` under an `envs.<type>` table,
    which takes precedence over it — so both spellings are matched. `check_repo_revision` is the
    real backstop; this only turns a silent wrong-code run into an early, explicable failure.
    """
    if not re.search(r"^\s*clone(-repos)?\s*=\s*true", run([dda(), "config", "show"]).stdout, re.MULTILINE):
        return

    config_path = run([dda(), "config", "find"]).stdout.strip()
    fail(
        UNUSABLE_CHECKOUT,
        f"Repository cloning is enabled in {config_path or 'your dda config'}, so a new dev env\n"
        "would clone datadog-agent from GitHub instead of mounting this checkout, and the tests\n"
        "would not exercise your changes. Turn it off — this only affects envs created afterwards:\n"
        "    dda config set env.dev.clone-repos false\n"
        "If that is already false, an `envs` table in the same file sets `clone` directly; remove\n"
        "it. Then run this script again.",
    )


def env_state(instance: str) -> str:
    """The dev env's state, as `dda env dev status` reports it (`State: <state>`)."""
    result = run(env_command(instance, "status"))
    match = re.search(r"State:\s*(\S+)", result.stdout)
    return match.group(1).lower() if match else "unknown"


def ensure_started(instance: str, root: Path, mounts: list[tuple[Path, str]]) -> None:
    state = env_state(instance)
    if state == "started":
        print(f"Dev env `{instance}` is already started.", file=sys.stderr)
        return

    if state == "stopped":
        # A stopped env keeps its saved configuration and refuses new mount options, so
        # resume it as-is. Its Pulumi state may still matter, so never remove it here.
        print(f"Resuming dev env `{instance}`...", file=sys.stderr)
        if run(env_command(instance, "start"), cwd=root, capture=False).returncode != 0:
            fail(ERROR, f"Could not resume dev env `{instance}`. See /.agents/skills/run-e2e/references/devenv.md.")
        return

    if state == "error":
        # `error` is any container that exited non-zero, which covers both "never finished
        # starting" and "ran tests, then died" — and the second holds the only copy of those
        # stacks' state. Nothing can distinguish them while the container is down, so removing
        # it automatically would risk orphaning live resources, the very thing `down` refuses
        # to do. Leave the decision with the user.
        fail(
            STACK_STILL_LIVE,
            f"Dev env `{instance}` is in an error state, so it cannot be prepared or inspected.\n"
            "Its container exited: either it never finished starting, or it ran tests and died\n"
            "afterwards — in which case it holds the only copy of their Pulumi state, and removing\n"
            "it would orphan whatever those stacks still have running. That cannot be checked from\n"
            "outside. If you know it never got as far as provisioning, recreate it:\n"
            f"    python {SCRIPT} down --id {instance} --force\n"
            f"    python {SCRIPT} up --id {instance}",
        )

    if state != "nonexistent":
        # `starting`, `stopping` or a state `dda env dev status` did not report in a form we parse.
        fail(ERROR, f"Dev env `{instance}` is in state `{state}`; wait for it to settle and retry.")

    check_clone_repos_disabled()

    command = env_command(instance, "start", "--no-pull")
    for host_path, container_path in mounts:
        # Read-only so a test run inside the container cannot corrupt the host's keypair.
        # as_posix() keeps a Windows drive path parseable as a docker -v spec.
        command += ["-v", f"{host_path.as_posix()}:{container_path}:ro"]

    print(f"Starting dev env `{instance}` (this boots a container)...", file=sys.stderr)
    # cwd matters: the repo bind mount is derived from the current directory's parent.
    if run(command, cwd=root, capture=False).returncode != 0:
        fail(ERROR, f"`dda env dev start` failed for `{instance}`. See /.agents/skills/run-e2e/references/devenv.md.")


def check_repo_revision(instance: str, root: Path) -> None:
    """Confirm the container's checkout is this working tree and not something else.

    An env created while `env.dev.clone-repos` was set holds a shallow clone of the default branch
    instead of a mount, and testing that silently exercises the wrong code.

    Matching revisions alone does not prove it is a mount: a clone sitting on an up-to-date default
    branch has the same HEAD as a host checked out there, while none of the host's uncommitted work
    exists in it. `--is-shallow-repository` settles the question, because `dda env dev start` clones
    with `--depth 1` and a bind-mounted clone is never shallow. Both values come back from one
    `git rev-parse`, so this costs no extra round trip. Comparing `git status --porcelain` as well
    would also catch a hand-built full clone, but nothing here produces that state and it would cost
    a second call into the container.
    """
    revision_args = ("rev-parse", "HEAD", "--is-shallow-repository")
    host = run(["git", *revision_args], cwd=root).stdout.split()
    result = run(env_command(instance, "run", "--", "git", "-C", CONTAINER_REPO, *revision_args))
    # Tolerate any leading output from the wrapper by taking the last two fields rather than the
    # first. Whether it ever emits any has not been established — `live_stacks` parses this same
    # wrapper's stdout as JSON without stripping anything, and that works — so read this as defensive
    # slicing, not as a documented property of `dda env dev run`.
    container, shallow = (result.stdout.split()[-2:] + ["", ""])[:2] if result.returncode == 0 else ("", "")
    host_revision = host[0] if host else ""

    if container == host_revision and shallow == "false":
        return

    if container == host_revision:
        fail(
            UNUSABLE_CHECKOUT,
            f"The dev env's {CONTAINER_REPO} is a shallow clone rather than a mount of {root}, even\n"
            "though both are at the same revision — so none of your uncommitted work is in it.\n"
            "Recreate the env so it mounts this tree:\n"
            "    dda config set env.dev.clone-repos false\n"
            f"    python {SCRIPT} down --id {instance} --force\n"
            f"    python {SCRIPT} up --id {instance}\n"
            "Do that only once you are sure the env holds no live Pulumi stacks.",
        )

    # git itself may have refused rather than the revision being wrong — a uid mismatch on the
    # bind mount produces `detected dubious ownership`, which needs a different fix.
    git_error = result.stderr.strip() or result.stdout.strip()
    detail = "" if result.returncode == 0 else f"\ngit in the container said:\n{git_error}"
    fail(
        UNUSABLE_CHECKOUT,
        f"The dev env's {CONTAINER_REPO} is not this working tree: it is at\n"
        f"    {container or '<no revision - the checkout is missing, empty or unreadable>'}\n"
        f"while {root} is at\n"
        f"    {host_revision}\n"
        "Tests there would not exercise your changes. Unless the output below points elsewhere,\n"
        "recreate the env so it mounts this tree:\n"
        f"    python {SCRIPT} down --id {instance} --force\n"
        f"    python {SCRIPT} up --id {instance}\n"
        f"Do that only once you are sure the env holds no live Pulumi stacks.{detail}",
    )


def install_config(instance: str) -> None:
    """Copy the mounted E2E config into the container's home directory.

    Both the Go runner and the invoke preflight look for it at `$HOME/.test_infra_config.yaml`
    and neither accepts an override, but it cannot be mounted there: the entrypoint chowns all
    of `$HOME`, which fails on a read-only mount. Copying from the read-only mount keeps the
    host's file untouchable while giving the container a writable copy of its own, and re-running
    it on every `up` keeps that copy in step with the host.

    `dda env dev fs import` would be the obvious tool, but it quotes its paths in a way that the
    nu shell passes through as part of the filename, so it fails outright when the env's shell is
    nu. A plain `install` of arguments we control avoids the problem.
    """
    print("Installing the E2E config in the dev env...", file=sys.stderr)
    # `install -m` rather than a copy followed by a chmod: it is one round trip instead of two,
    # and it never leaves the file — which carries the Pulumi passphrase and the Datadog API key —
    # world-readable in between, which a copy off a Windows filesystem otherwise does.
    command = env_command(
        instance, "run", "--", "install", "-m", "600", f"{CONTAINER_KEY_DIR}/{CONFIG_NAME}", CONTAINER_CONFIG
    )
    if run(command).returncode != 0:
        fail(ERROR, f"Could not install the E2E config in `{instance}`.")


def ensure_pulumi_backend(instance: str) -> None:
    """Select Pulumi's local backend and install its plugins inside the container.

    The binary already comes from the base image, but $HOME is not a persistent volume,
    so ~/.pulumi has to be re-established for each container.
    """
    if run(env_command(instance, "run", "--", "pulumi", "whoami")).returncode == 0:
        print("Pulumi backend already configured.", file=sys.stderr)
        return

    print("Configuring Pulumi backend and plugins in the dev env...", file=sys.stderr)
    # --no-interactive keeps this to Pulumi only. The interactive path would run as the
    # container user `dd` and mint a second AWS keypair, which then makes every later
    # fresh container fail; see /.agents/skills/run-e2e/references/setup.md.
    command = env_command(
        instance,
        "run",
        "--",
        "env",
        # Otherwise a newer upstream Pulumi is treated as "not installed" and re-downloaded
        # into the ephemeral home, where the image's copy shadows it anyway.
        "PULUMI_SKIP_UPDATE_CHECK=true",
        "dda",
        "inv",
        "--",
        "e2e.setup",
        "--no-interactive",
    )
    if run(command, capture=False).returncode != 0:
        fail(
            ERROR,
            "Failed to configure Pulumi in the dev env. See /.agents/skills/run-e2e/references/troubleshooting.md.",
        )


def check_aws(instance: str) -> None:
    """Confirm the container can assume the sandbox role before a test run depends on it.

    Asking the container is the only way to know. The host's `~/.aws` is shared with it, but the
    framework authenticates through a profile whose credential_process runs aws-vault, and
    aws-vault keeps its tokens in its own keyring rather than in that shared directory — so the
    host's session state says nothing about whether this will work, and a healthy host session
    cannot prime the container.

    Consequently this call is not passive: with no usable token in that keyring, aws-vault starts
    an SSO flow and the env's browser proxy opens it on the user's desktop. Hence the warning
    before it and the timeout after it — unattended, there is nobody to accept the prompt. The
    timeout reaps the local process only, so an `aws` left behind inside the container may briefly
    contend with the next attempt.
    """
    print(
        "Checking AWS access from the dev env. If its credential helper has no token yet, this\n"
        "opens an AWS SSO tab on your desktop that you need to accept before it can continue.",
        file=sys.stderr,
    )
    result = run(
        env_command(instance, "run", "--", "aws", "sts", "get-caller-identity", "--profile", EXEC_PROFILE),
        timeout=180,
    )
    if result.returncode == 0:
        return
    fail(
        NO_AWS_SESSION,
        "The dev env cannot authenticate to AWS. Its credential helper keeps tokens in a keyring\n"
        "local to the container, which cannot be primed from the host, so authorize it there —\n"
        "this opens a browser tab through the env's proxy and waits for you to accept it:\n"
        f"    dda env dev run -t {ENV_TYPE} --id {instance} -- aws-vault login {SSO_PROFILE}\n"
        "Then run this script again.\n"
        f"\naws sts said:\n{result.stderr.strip() or result.stdout.strip()}",
    )


def live_stacks(instance: str) -> list[str] | None:
    """Stacks the container still knows about, or None if they could not be listed."""
    result = run(
        env_command(instance, "run", "--", "pulumi", "stack", "ls", "--all", "--project", PULUMI_PROJECT, "--json")
    )
    if result.returncode != 0:
        return None
    try:
        return [stack["name"] for stack in json.loads(result.stdout) if "name" in stack]
    except (json.JSONDecodeError, TypeError):
        return None


def command_up(args: argparse.Namespace) -> None:
    if in_devenv():
        fail(
            ALREADY_IN_DEVENV,
            "Already inside a dev env — run `dda inv -- new-e2e-tests.run` directly instead of\n"
            "nesting another environment.",
        )

    if yaml is None:
        fail(ERROR, "PyYAML is required. Install it, or run this script with the interpreter `dda` uses.")

    root = resolve_repo_root()
    config, keys = load_host_config()
    container_config = write_container_config(config, args.id, keys)

    mounts = [(container_config, f"{CONTAINER_KEY_DIR}/{CONFIG_NAME}")]
    mounts += [(path, container_key(section, path)) for (section, _), path in keys.items()]
    ensure_started(args.id, root, mounts)
    check_repo_revision(args.id, root)
    install_config(args.id)
    ensure_pulumi_backend(args.id)
    if args.no_aws_check:
        # A locally-provisioned target needs no AWS *session*, and the check below costs an
        # interactive SSO acceptance, so this lets someone whose session has lapsed still run one.
        # It does not remove the need for AWS *config*: both `load_host_config` above and the run
        # task's own preflight require `configParams.aws.keyPairName` whatever the target is. The
        # caller has to opt out because `up` never sees the target.
        print("Skipping the AWS access check; the run will fail late if the target needs it.", file=sys.stderr)
    else:
        check_aws(args.id)

    # Everything else the run needs is in the config copy. This one has no equivalent there, and
    # being slash-free it survives Git Bash's path conversion on a Windows host.
    env_args = [f"E2E_STACK_NAME_SUFFIX={stack_suffix(args.stack_name_suffix)}"]
    # Spelled `dda` rather than the resolved executable path, since this gets pasted into a shell.
    run_prefix = env_command(args.id, "run", "--", "env", *env_args, executable="dda")

    if args.json:
        print(json.dumps({"run_prefix": run_prefix}, indent=2))
    else:
        print("Dev env ready. Run tests with:")
        print(" ".join(run_prefix) + " dda inv -- new-e2e-tests.run --targets=<target>")


def removal_blocker(instance: str, state: str) -> str | None:
    """Why this env should not be removed yet, or None if it is safe to."""
    if state == "error":
        # `up` cannot resume this state either, so do not send the reader there — that would be a
        # loop. Their only options are to decide it holds nothing, or to salvage it by hand.
        return (
            f"Dev env `{instance}` is `error`, so its Pulumi stacks cannot be checked and it cannot\n"
            "be started to check them. Pass --force once you are satisfied it never provisioned\n"
            "anything, or recover its state by hand first."
        )

    if state != "started":
        # Nothing is listening, so the stacks cannot be checked, but a stopped env can be resumed.
        return (
            f"Dev env `{instance}` is `{state}`, so its Pulumi stacks cannot be checked. Start it\n"
            f"with `python {SCRIPT} up --id {instance}` to check them, or pass --force."
        )

    stacks = live_stacks(instance)
    if stacks is None:
        return (
            f"Could not list Pulumi stacks in `{instance}`, so it is not safe to remove: any stack\n"
            "state it holds exists nowhere else. Inspect it with\n"
            f"    dda env dev shell -t {ENV_TYPE} --id {instance}\n"
            "or force removal with --force once you are sure nothing is running."
        )
    if stacks:
        return (
            f"Dev env `{instance}` still has {len(stacks)} stack(s): {', '.join(stacks)}.\n"
            "Its ~/.pulumi is not a persistent volume, so removing it now would orphan whatever\n"
            "those stacks still hold in the cloud. Destroy them first:\n"
            f"    dda env dev run -t {ENV_TYPE} --id {instance} -- dda inv -- new-e2e-tests.clean -s"
        )
    return None


def command_down(args: argparse.Namespace) -> None:
    state = env_state(args.id)
    if state == "nonexistent":
        print(f"Dev env `{args.id}` does not exist; nothing to remove.", file=sys.stderr)
        return

    blocker = removal_blocker(args.id, state)
    if blocker:
        if not args.force:
            fail(STACK_STILL_LIVE, blocker)
        print(blocker, file=sys.stderr)

    # `stop --remove` is the only removal `dda` accepts while the container is running, and
    # `remove` the only one it accepts once it is not.
    removal = ("stop", "--remove") if state == "started" else ("remove",)
    print(f"Removing dev env `{args.id}` (state `{state}`)...", file=sys.stderr)
    if run(env_command(args.id, *removal), capture=False).returncode != 0:
        fail(ERROR, f"Failed to remove dev env `{args.id}`.")
    # The host-side config copy only exists to be mounted into the env that is now gone.
    shutil.rmtree(container_config_dir(args.id), ignore_errors=True)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    subparsers = parser.add_subparsers(dest="command", required=True)

    up = subparsers.add_parser("up", help="start and prepare a dev env for E2E runs (idempotent)")
    up.add_argument("--id", default="e2e-run", help="dev env identifier (default: e2e-run)")
    up.add_argument("--json", action="store_true", help="emit machine-readable output")
    up.add_argument("--stack-name-suffix", help="combined with the host username into E2E_STACK_NAME_SUFFIX")
    up.add_argument(
        "--no-aws-check",
        action="store_true",
        help="skip the AWS access check for a locally-provisioned target; AWS config is still required",
    )
    up.set_defaults(func=command_up)

    down = subparsers.add_parser("down", help="remove a dev env, refusing while stacks are live")
    down.add_argument("--id", default="e2e-run", help="dev env identifier (default: e2e-run)")
    down.add_argument("--force", action="store_true", help="remove even if stacks are live or unknown")
    down.set_defaults(func=command_down)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
