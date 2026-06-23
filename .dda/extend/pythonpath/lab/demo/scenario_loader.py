# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Auto-discovery and CLI generation for demo scenarios.

Scans test/new-e2e/tests/*/scenario.py and generates, for each team and
each env-type declared in DEMO_ENVS:

    dda lab demo <team> <env>   create  [--api-key] [--id] [<create_options>...]
    dda lab demo <team> <env>   run     --scenario <name> --action <action> [--id]

To delete a demo environment use: dda lab delete --id <id>

Teams declare DEMO_ENVS in their scenario.py:

    DEMO_ENVS = {
        "aws": DemoEnv(
            pulumi_scenario="aws/agent-health-demo",
            description="AWS EC2 + Docker",
            create_options=[DemoOption(...)],
            scenarios={"docker-permissions": Scenario(...)},
        ),
        "kind": DemoEnv(...),
    }
"""

from __future__ import annotations

import glob
import importlib.util
import json
import sys
import types
from pathlib import Path
from typing import Any

import click
from dda.cli.base import DynamicGroup

# ---------------------------------------------------------------------------
# Repo root / module loading
# ---------------------------------------------------------------------------


_MODULE_PREFIX = "github.com/DataDog/datadog-agent"


def _find_repo_root() -> Path:
    """Walk upward until we find the go.mod that declares the agent module."""
    here = Path(__file__).resolve()
    candidate = here
    while candidate != candidate.parent:
        go_mod = candidate / "go.mod"
        if go_mod.exists() and _MODULE_PREFIX in go_mod.read_text():
            return candidate
        candidate = candidate.parent
    raise FileNotFoundError(f"Could not find repo root containing module {_MODULE_PREFIX!r}")


def _ensure_pythonpath() -> None:
    pythonpath = str(Path(__file__).resolve().parent.parent.parent)
    if pythonpath not in sys.path:
        sys.path.insert(0, pythonpath)


def _load_module(team_dir: str, repo_root: Path) -> types.ModuleType | None:
    scenario_path = repo_root / "test" / "new-e2e" / "tests" / team_dir / "scenario.py"
    if not scenario_path.exists():
        return None
    _ensure_pythonpath()
    spec = importlib.util.spec_from_file_location(f"_demolab_scenario_{team_dir}", scenario_path)
    if spec is None or spec.loader is None:
        return None
    module = importlib.util.module_from_spec(spec)
    try:
        spec.loader.exec_module(module)  # type: ignore[union-attr]
    except Exception as exc:
        import traceback

        print(f"[demolab] Warning: failed to load {scenario_path}: {exc}", flush=True)
        traceback.print_exc()
        return None
    return module


def _discover_teams(repo_root: Path) -> list[str]:
    return sorted(
        Path(p).parent.name for p in glob.glob(str(repo_root / "test" / "new-e2e" / "tests" / "*" / "scenario.py"))
    )


# ---------------------------------------------------------------------------
# SSH helper
# ---------------------------------------------------------------------------


def _ensure_key_in_agent(app: Any, ssh_key_path: str, private_key_password: str) -> None:
    """Load an encrypted SSH key into ssh-agent non-interactively via SSH_ASKPASS.

    BatchMode=yes suppresses all passphrase prompts, so an encrypted key that
    is not yet in the agent causes silent auth failure.  This loads the key
    once; subsequent ssh calls in the same session reuse the agent entry.
    """
    import os
    import shlex
    import stat
    import tempfile

    from dda.utils.process import EnvVars

    # Skip if the key fingerprint is already listed by the agent.
    listed = app.subprocess.attach(["ssh-add", "-l"], capture_output=True, text=True, check=False)
    if listed.returncode == 0 and ssh_key_path in (listed.stdout or ""):
        return

    # Write a minimal askpass script that prints the passphrase to stdout.
    with tempfile.NamedTemporaryFile(mode="w", suffix=".sh", delete=False) as f:
        f.write(f"#!/bin/sh\nprintf '%s' {shlex.quote(private_key_password)}\n")
        askpass = f.name
    os.chmod(askpass, stat.S_IRWXU)
    try:
        app.subprocess.attach(
            ["ssh-add", ssh_key_path],
            env=EnvVars({"SSH_ASKPASS": askpass, "SSH_ASKPASS_REQUIRE": "force", "DISPLAY": "dummy"}),
            capture_output=True,
            text=True,
            check=False,
        )
    finally:
        os.unlink(askpass)


def _run_ssh_commands(
    app: Any,
    host_ip: str,
    ssh_user: str,
    commands: list[str],
    ssh_key_path: str | None = None,
    private_key_password: str | None = None,
) -> None:
    if ssh_key_path and private_key_password:
        _ensure_key_in_agent(app, ssh_key_path, private_key_password)
    key_args = ["-i", ssh_key_path] if ssh_key_path else []
    for cmd in commands:
        result = app.subprocess.attach(
            ["ssh", "-o", "StrictHostKeyChecking=no", "-o", "BatchMode=yes"]
            + key_args
            + [f"{ssh_user}@{host_ip}", cmd],
            capture_output=True,
            text=True,
            check=False,
        )
        if result.returncode != 0:
            app.abort(f"Command failed on {host_ip}:\n  {cmd}\n{result.stderr}")
        if result.stdout:
            app.display_info(result.stdout.rstrip())


# ---------------------------------------------------------------------------
# Top-level demo group  →  per-team group  →  per-env group
# ---------------------------------------------------------------------------


def create_demo_group() -> DynamicGroup:
    """
    Top-level group that auto-discovers all teams with a scenario.py and
    generates a subgroup per team.
    """
    repo_root = _find_repo_root()

    class DemoGroup(DynamicGroup):
        def list_commands(self, ctx: click.Context) -> list[str]:
            return _discover_teams(repo_root)

        def get_command(self, ctx: click.Context, cmd_name: str) -> click.Command | None:
            return _make_team_group(cmd_name, repo_root)

    @click.group(
        cls=DemoGroup, invoke_without_command=True, short_help="Demo environments and scenarios for agent teams"
    )
    @click.option("--list", "list_teams", is_flag=True, help="List available teams and exit.")
    @click.pass_context
    def demo_group(ctx: click.Context, list_teams: bool) -> None:
        """
        Demo environments and scenarios for agent teams.

        Each team with a test/new-e2e/tests/<team>/scenario.py appears as a
        subcommand. Each env-type declared in DEMO_ENVS gets its own
        create + scenario retrigger/remediate commands.
        """
        if list_teams or ctx.invoked_subcommand is None:
            for team in _discover_teams(repo_root):
                click.echo(f"  {team}")
            ctx.exit()

    return demo_group


def _make_team_group(team_dir: str, repo_root: Path) -> click.Group | None:
    """Generate the per-team group containing one subgroup per env-type."""
    module = _load_module(team_dir, repo_root)
    if module is None:
        return None

    demo_envs: dict[str, Any] = getattr(module, "DEMO_ENVS", {})
    if not demo_envs:
        return None

    class TeamGroup(DynamicGroup):
        def list_commands(self, ctx: click.Context) -> list[str]:
            return list(demo_envs.keys())

        def get_command(self, ctx: click.Context, cmd_name: str) -> click.Command | None:
            demo_env = demo_envs.get(cmd_name)
            if demo_env is None:
                return None
            return _make_env_group(team_dir, cmd_name, demo_env)

    @click.group(cls=TeamGroup, invoke_without_command=True, short_help=f"Demo environments for {team_dir}")
    @click.option("--list", "list_envs", is_flag=True, help="List available env types and exit.")
    @click.pass_context
    def team_group(ctx: click.Context, list_envs: bool) -> None:
        """Demo environments and scenarios for the {team_dir} team."""
        if list_envs or ctx.invoked_subcommand is None:
            for env_name, demo_env in demo_envs.items():
                click.echo(f"  {env_name}  —  {demo_env.description}")
            ctx.exit()

    team_group.__doc__ = f"Demo environments and scenarios for the {team_dir} team."
    team_group.name = team_dir
    return team_group


# ---------------------------------------------------------------------------
# Per-env group: create + scenario subcommands
# ---------------------------------------------------------------------------


def _make_env_group(team_dir: str, env_name: str, demo_env: Any) -> click.Group:
    """Generate the per-env group: create, run."""

    class EnvGroup(DynamicGroup):
        def list_commands(self, ctx: click.Context) -> list[str]:
            cmds = ["create"]
            if demo_env.scenarios:
                cmds += ["run"]
            return cmds

        def get_command(self, ctx: click.Context, cmd_name: str) -> click.Command | None:
            if cmd_name == "create":
                return _make_create_command(team_dir, env_name, demo_env)
            if cmd_name == "run" and demo_env.scenarios:
                return _make_run_command(demo_env, team_dir, env_name)
            return None

    @click.group(cls=EnvGroup, short_help=demo_env.description)
    def env_group() -> None:
        pass

    env_group.__doc__ = demo_env.description
    env_group.name = env_name
    return env_group


# ---------------------------------------------------------------------------
# create command
# ---------------------------------------------------------------------------


def _make_create_command(team_dir: str, env_name: str, demo_env: Any) -> click.Command:
    from dda.cli.base import dynamic_command, pass_app

    base_id = f"demo-{team_dir}-{env_name}"

    @pass_app
    def _create_impl(app: Any, *, api_key: str | None, id: str | None, **kwargs: Any) -> None:
        import os

        from dda.utils.process import EnvVars

        from lab import LabEnvironment
        from lab.config import load_config as _load_config

        infra_config = _load_config()

        if api_key is None:
            api_key = infra_config.get_api_key()
        if not api_key:
            app.abort("No API key provided. Use --api-key or set E2E_API_KEY.")

        # Derive stack ID from the scenario name when not explicitly set.
        for_scenario = kwargs.get("scenario")
        if id is None:
            id = f"{base_id}-{for_scenario}" if for_scenario else base_id

        # Export the stored Pulumi passphrase when not already set, so users following
        # the standard dda inv e2e.setup flow don't need PULUMI_CONFIG_PASSPHRASE in
        # their shell (mirrors tasks/new_e2e_tests.py lines 621-628).
        passphrase = infra_config.pulumi.passphrase
        pulumi_env = (
            EnvVars({"PULUMI_CONFIG_PASSPHRASE": passphrase})
            if passphrase and "PULUMI_CONFIG_PASSPHRASE" not in os.environ
            else None
        )

        repo_root = _find_repo_root()
        pulumi_dir = str(repo_root / "test" / "new-e2e" / "run")

        verb = "Updating" if LabEnvironment.exists(app, id) else "Provisioning"
        app.display_info(f"{verb} {team_dir}/{env_name} demo environment '{id}' ...")

        # Create the Pulumi stack if it does not already exist.  pulumi config set
        # and pulumi up both require the stack to exist first.
        init_result = app.subprocess.attach(
            ["pulumi", "stack", "init", "--no-select", id, "-C", pulumi_dir],
            capture_output=True,
            text=True,
            check=False,
            **({"env": pulumi_env} if pulumi_env else {}),
        )
        # Ignore "already exists" (exit 1 with that message) — any other failure is fatal.
        if init_result.returncode != 0 and "already exists" not in (init_result.stderr or ""):
            app.abort(f"Failed to initialise Pulumi stack '{id}': {(init_result.stderr or '').strip()}")

        def _set_secret(key: str, value: str) -> None:
            r = app.subprocess.attach(
                ["pulumi", "config", "set", "--secret", key, "-s", id, "-C", pulumi_dir],
                input=value,
                capture_output=True,
                text=True,
                check=False,
                **({"env": pulumi_env} if pulumi_env else {}),
            )
            if r.returncode != 0:
                app.abort(f"Failed to set Pulumi secret '{key}'.")

        # Secrets are set via stdin so values are never visible in process argv or CI logs.
        _set_secret("ddagent:apiKey", api_key)

        aws = infra_config.aws
        if aws.private_key_password:
            _set_secret("ddinfra:aws/defaultPrivateKeyPassword", aws.private_key_password)

        config_args = [
            "-c",
            f"scenario={demo_env.pulumi_scenario}",
            "-c",
            f"demolab:teamDir={team_dir}",
            "-c",
            f"demolab:envName={env_name}",
        ]

        # Forward non-sensitive AWS infra config from ~/.test_infra_config.yaml.
        if aws.key_pair_name:
            config_args += ["-c", f"ddinfra:aws/defaultKeyPairName={aws.key_pair_name}"]
        if aws.public_key_path:
            config_args += ["-c", f"ddinfra:aws/defaultPublicKeyPath={aws.public_key_path}"]
        if aws.private_key_path:
            config_args += ["-c", f"ddinfra:aws/defaultPrivateKeyPath={aws.private_key_path}"]

        for opt in demo_env.create_options:
            value = kwargs.get(opt.name)
            if value is not None:
                for key in opt.pulumi_keys:
                    config_args += ["-c", f"{key}={value}"]

        if for_scenario:
            if for_scenario not in demo_env.scenarios:
                app.abort(f"Unknown scenario '{for_scenario}'. " f"Available: {', '.join(demo_env.scenarios)}")
                return
            config_args += ["-c", f"demolab:demoScenario={for_scenario}"]
            for key, value in demo_env.scenarios[for_scenario].create_defaults.items():
                config_args += ["-c", f"{key}={value}"]

        # Save a minimal record before pulumi up so the stack is trackable
        # even if provisioning fails partway through.
        site = kwargs.get("site", "")
        metadata: dict[str, Any] = {
            "ssh_user": "ubuntu",
            "stack": id,
            "pulumi_dir": pulumi_dir,
            "demo_env": env_name,
        }
        if for_scenario:
            metadata["scenario"] = for_scenario
        if site:
            metadata["site"] = site
        LabEnvironment(app, name=id, env_type=team_dir, category="demo", metadata=metadata).save()

        exit_code = app.subprocess.run(
            ["pulumi", "up", "--yes", "-s", id, "-C", pulumi_dir] + config_args,
            check=False,
            env=pulumi_env,
        )
        if exit_code != 0:
            app.abort(f"pulumi up failed (exit {exit_code}). " f"Run 'dda lab delete --id {id}' to clean up.")

        # Read the stable hostIP output exported by the provisioner.
        out = app.subprocess.attach(
            ["pulumi", "stack", "output", "--json", "-s", id, "-C", pulumi_dir],
            capture_output=True,
            text=True,
            check=False,
            **({"env": pulumi_env} if pulumi_env else {}),
        )
        host_ip = None
        if out.returncode == 0 and out.stdout.strip():
            try:
                outputs = json.loads(out.stdout)
                host_ip = outputs.get("hostIP")
                if not host_ip:
                    # Fall back to the standard e2e-framework output shape:
                    # dd-Host-<name>.address (used by ec2.VMRun and similar provisioners).
                    for key, val in outputs.items():
                        if key.startswith("dd-Host-") and isinstance(val, dict):
                            host_ip = val.get("address")
                            if host_ip:
                                break
                if not host_ip and demo_env.host_required:
                    app.display_warning(
                        "Stack output 'hostIP' is absent — check that the provisioner "
                        "calls ctx.Export(\"hostIP\", ...). retrigger/remediate will not work."
                    )
            except (json.JSONDecodeError, AttributeError):
                if demo_env.host_required:
                    app.display_warning("Could not parse stack outputs; retrigger/remediate may not work.")
        elif out.returncode == 0 and not out.stdout.strip() and demo_env.host_required:
            app.display_warning("pulumi stack output returned no data; hostIP may not be set.")

        if host_ip:
            metadata["host_ip"] = host_ip
        LabEnvironment(app, name=id, env_type=team_dir, category="demo", metadata=metadata).save()

        app.display_success(f"Environment '{id}' created.")
        if host_ip:
            app.display_info(f"  Host IP : {host_ip}")
            app.display_info(f"  SSH     : ssh ubuntu@{host_ip}")
            if site:
                app.display_info(f"  Datadog : https://app.{site}")
        else:
            app.display_info("  (Could not read host IP from stack outputs)")

    fn = _create_impl
    fn = click.option(
        "--id",
        "-i",
        default=None,
        help=f"Environment id (default: {base_id}[-<scenario>])",
    )(fn)
    fn = click.option("--api-key", default=None, envvar="E2E_API_KEY", help="Datadog API key")(fn)
    fn = click.option(
        "--scenario",
        "-s",
        default=None,
        help="Scenario this VM will be used for; applies its create_defaults to the Pulumi config.",
    )(fn)
    for opt in reversed(demo_env.create_options):
        cli_name = f"--{opt.name.replace('_', '-')}"
        fn = click.option(
            cli_name,
            default=opt.default,
            required=opt.required,
            show_default=opt.show_default and opt.default is not None,
            help=opt.help,
        )(fn)

    fn = dynamic_command(short_help=f"Provision a {team_dir} {env_name} demo environment")(fn)
    fn.__doc__ = (
        f"Provision a {team_dir} demo environment using {env_name}.\n\n"
        "The API key is resolved from --api-key, E2E_API_KEY, or ~/.test_infra_config.yaml."
    )
    fn.name = "create"
    return fn


# ---------------------------------------------------------------------------
# run command — dispatches to a named action defined in scenario.actions
# ---------------------------------------------------------------------------


def _make_run_command(demo_env: Any, team_dir: str, env_name: str) -> click.Command:
    from dda.cli.base import dynamic_command, pass_app

    scenario_names = sorted(demo_env.scenarios.keys())
    default_scenario = scenario_names[0] if len(scenario_names) == 1 else None
    scenarios_help = ", ".join(scenario_names)

    @dynamic_command(short_help="Run a named action for a scenario (e.g. retrigger, reset)")
    @click.option("--scenario", "-s", default=default_scenario, help=f"Scenario name. Available: {scenarios_help}")
    @click.option("--action", "-a", default=None, help="Action to run (defined per scenario in scenario.py)")
    @click.option("--list", "list_scenarios", is_flag=True, help="List available scenarios and their actions.")
    @click.option("--id", "-i", default=None, help=f"Environment id (default: first {team_dir}/{env_name} env)")
    @pass_app
    def cmd(app: Any, *, scenario: str | None, action: str | None, list_scenarios: bool, id: str | None) -> None:
        if list_scenarios:
            for name, sc in demo_env.scenarios.items():
                actions_help = ", ".join(sc.actions.keys())
                app.display_info(f"  {name}  —  {sc.description}  [actions: {actions_help}]")
            return

        if not scenario:
            app.abort(f"--scenario/-s is required. Available: {scenarios_help}")

        sc = demo_env.scenarios.get(scenario)
        if sc is None:
            app.abort(f"Unknown scenario '{scenario}'. Available: {scenarios_help}")

        if not action:
            actions_help = ", ".join(sc.actions.keys())
            app.abort(f"--action/-a is required. Available: {actions_help}")

        if action not in sc.actions:
            actions_help = ", ".join(sc.actions.keys())
            app.abort(f"Unknown action '{action}' for scenario '{scenario}'. Available: {actions_help}")

        _run_action(app, id, sc, scenario, team_dir, env_name, action)

    cmd.name = "run"
    return cmd


def _run_action(
    app: Any, env_id: str | None, scenario: Any, scenario_name: str | None, team_dir: str, env_name: str, action: str
) -> None:
    from lab import LabEnvironment

    if env_id is None:
        envs = [
            e
            for e in LabEnvironment.load_all(app, env_type=team_dir)
            if e.metadata.get("demo_env") == env_name
            and (scenario_name is None or e.metadata.get("scenario") in (None, scenario_name))
        ]
        if not envs:
            app.abort(
                f"No {team_dir}/{env_name} demo environments found. "
                f"Run 'dda lab demo {team_dir} {env_name} create' first."
            )
        # Sort by created_at descending so the most recent environment is preferred.
        envs.sort(key=lambda e: e.created_at, reverse=True)
        if len(envs) > 1:
            app.display_warning(
                f"Multiple {team_dir}/{env_name} environments found; using the most recent ('{envs[0].name}'). "
                f"Use --id to select a specific one."
            )
        env = envs[0]
    else:
        env = LabEnvironment.load(app, env_id)
        if env is None:
            app.abort(f"Environment '{env_id}' not found.")
            return

    host_ip = env.metadata.get("host_ip")
    ssh_user = env.metadata.get("ssh_user", "ubuntu")
    if not host_ip:
        app.abort(f"Environment '{env.name}' has no host_ip in metadata.")

    from lab.config import load_config as _load_config

    _aws_cfg = _load_config().aws
    ssh_key_path = _aws_cfg.private_key_path
    private_key_password = _aws_cfg.private_key_password

    act = scenario.actions[action]
    app.display_info(f"Running '{action}' for issue '{scenario.issue}' on {host_ip}...")
    _run_ssh_commands(
        app, host_ip, ssh_user, act.commands, ssh_key_path=ssh_key_path, private_key_password=private_key_password
    )
    app.display_success(act.message)
