from __future__ import annotations

import json
from typing import TYPE_CHECKING, Any

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


def _recover_orphaned_stacks(app: Application, pulumi_dir: str) -> None:
    """Scan Pulumi stacks for demo stacks with no local record and recreate them."""
    from lab import LabEnvironment

    result = app.subprocess.attach(
        ["pulumi", "stack", "ls", "--json", "-C", pulumi_dir],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0 or not result.stdout.strip():
        return

    try:
        stacks = json.loads(result.stdout)
    except json.JSONDecodeError:
        return

    known = {env.metadata.get("stack") for env in LabEnvironment.load_all(app)}

    for stack in stacks:
        name = stack.get("name", "")
        if name in known:
            continue
        # Only consider stacks whose name starts with "demo" — this covers both
        # pattern-named stacks (demo-<team>-<env>) and custom-named ones (demotest,
        # demo-myenv) while excluding unrelated E2E test stacks.
        if not name.lower().startswith("demo"):
            continue

        out = app.subprocess.attach(
            ["pulumi", "stack", "output", "--json", "-s", name, "-C", pulumi_dir],
            capture_output=True,
            text=True,
            check=False,
        )
        outputs = {}
        if out.returncode == 0 and out.stdout.strip():
            try:
                outputs = json.loads(out.stdout)
            except json.JSONDecodeError:
                pass

        # Skip empty stacks — they have no live resources.
        if not outputs:
            continue

        # Prefer team/env stored in Pulumi config by the create command.
        # Fall back to parsing the stack name, then to generic defaults.
        def _pulumi_config_get(
            key: str, _app: Application = app, _name: str = name, _pulumi_dir: str = pulumi_dir
        ) -> str | None:
            r = _app.subprocess.attach(
                ["pulumi", "config", "get", key, "-s", _name, "-C", _pulumi_dir],
                capture_output=True,
                text=True,
                check=False,
            )
            return r.stdout.strip() if r.returncode == 0 and r.stdout.strip() else None

        team_dir = _pulumi_config_get("demolab:teamDir")
        env_name = _pulumi_config_get("demolab:envName")
        if not team_dir or not env_name:
            if name.startswith("demo-"):
                parts = name[len("demo-") :].rsplit("-", 1)
                team_dir = team_dir or (parts[0] if len(parts) == 2 else name)
                env_name = env_name or (parts[1] if len(parts) == 2 else "custom")
            else:
                team_dir = team_dir or name
                env_name = env_name or "custom"

        host_ip = outputs.get("hostIP")
        if not host_ip:
            for key, val in outputs.items():
                if key.startswith("dd-Host-") and isinstance(val, dict):
                    host_ip = val.get("address")
                    if host_ip:
                        break
        metadata: dict[str, Any] = {
            "ssh_user": "ubuntu",
            "stack": name,
            "pulumi_dir": pulumi_dir,
            "demo_env": env_name,
        }
        if host_ip:
            metadata["host_ip"] = host_ip

        app.display_warning(f"Orphaned stack '{name}' found — recreating local record.")
        LabEnvironment(app, name=name, env_type=team_dir, category="demo", metadata=metadata).save()


def _refresh_env(app: Application, env: Any) -> bool:
    """Sync a single environment's metadata against its Pulumi stack.

    Returns True if the record should be kept, False if the stack is gone
    and the record was deleted.
    """
    from lab import LabEnvironment

    stack = env.metadata.get("stack")
    pulumi_dir = env.metadata.get("pulumi_dir")
    if not stack or not pulumi_dir:
        return True

    result = app.subprocess.attach(
        ["pulumi", "stack", "output", "--json", "-s", stack, "-C", pulumi_dir],
        capture_output=True,
        text=True,
        check=False,
    )

    if result.returncode != 0:
        stderr = result.stderr or ""
        if "no stack named" in stderr or "not found" in stderr or "does not exist" in stderr:
            app.display_warning(f"Stack '{stack}' no longer exists — removing local record '{env.name}'.")
            LabEnvironment.load(app, env.name).delete()
            return False
        # Transient error (e.g. auth) — keep the record, warn the user.
        app.display_warning(f"Could not refresh '{env.name}': {stderr.strip()}")
        return True

    try:
        outputs = json.loads(result.stdout) if result.stdout.strip() else {}
    except json.JSONDecodeError:
        return True

    host_ip = outputs.get("hostIP")
    if not host_ip:
        for key, val in outputs.items():
            if key.startswith("dd-Host-") and isinstance(val, dict):
                host_ip = val.get("address")
                if host_ip:
                    break
    if host_ip and host_ip != env.metadata.get("host_ip"):
        env.metadata["host_ip"] = host_ip
        env.save()

    return True


@dynamic_command(short_help="List all lab active environments")
@click.option("--type", "-t", "env_type", default=None, help="Filter by environment type (e.g., kind, gke, eks)")
@click.option("--json", "as_json", is_flag=True, help="Output as JSON")
@click.option(
    "--refresh",
    "-r",
    is_flag=True,
    help="Sync each environment's metadata against its Pulumi stack and prune stale records.",
)
@pass_app
def cmd(app: Application, *, env_type: str | None, as_json: bool, refresh: bool) -> None:
    """
    List all lab environments.

    Examples:

        # List all environments
        dda lab list

        # List only kind environments
        dda lab list --type kind

        # Prune stale records and update host IPs
        dda lab list --refresh

        # Output as JSON for scripting
        dda lab list --json
    """
    from lab import LabEnvironment

    environments = LabEnvironment.load_all(app, env_type=env_type)

    if refresh:
        # First pass: prune/update existing records.
        environments = [env for env in environments if _refresh_env(app, env)]
        # Second pass: recover demo stacks that have no local record.
        try:
            from lab.demo.scenario_loader import _find_repo_root

            pulumi_dir = str(_find_repo_root() / "test" / "new-e2e" / "run")
            _recover_orphaned_stacks(app, pulumi_dir)
        except Exception:
            pass
        environments = LabEnvironment.load_all(app, env_type=env_type)

    if not environments:
        if as_json:
            app.output("[]")
            return
        if env_type:
            app.display_info(f"No {env_type} environments found.")
        else:
            app.display_info("No lab environments found.")
        return

    if as_json:
        output = [
            {
                "id": env.name,
                "type": env.env_type,
                "category": env.category,
                "created_at": env.created_at,
                "metadata": env.metadata,
            }
            for env in environments
        ]
        app.display(json.dumps(output, indent=2))
        return

    table = {}
    for env in environments:
        if env.category not in table:
            table[env.category] = {}
        if env.env_type not in table[env.category]:
            table[env.category][env.env_type] = {}
        table[env.category][env.env_type][env.name] = env.to_dict()
    app.display_table(table)
