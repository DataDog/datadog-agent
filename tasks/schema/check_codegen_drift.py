"""
Verify that regenerating the config settings code (`schema.codegen`) does not
change the Agent's runtime configuration defaults.
"""

import difflib
import json
import os
import sys
import tempfile

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import color_message

# The two configurations dumped by the `agent dumpconfig` command.
TARGETS = ["core", "system-probe"]


def _agent_bin():
    """Return the path to the built agent binary for the current platform."""
    if sys.platform == "win32":
        return os.path.join(".", "bin", "agent", "agent.exe")
    return os.path.join(".", "bin", "agent", "agent")


def _dump_config(ctx, agent_bin, target, out_path):
    """Dump the runtime config defaults for `target` to `out_path` as JSON."""
    result = ctx.run(f"{agent_bin} dumpconfig --target {target}", hide="out")
    with open(out_path, "w") as f:
        f.write(result.stdout)


def _diff_config(base_path, generated_path, label):
    """Return True if the two JSON config dumps are equal, otherwise print a
    unified diff and return False."""
    with open(base_path) as f:
        base = json.load(f)
    with open(generated_path) as f:
        generated = json.load(f)

    if base == generated:
        return True

    # Re-serialize with sorted keys so the diff only reflects real value
    # differences, not key ordering.
    base_lines = json.dumps(base, indent=2, sort_keys=True).splitlines()
    generated_lines = json.dumps(generated, indent=2, sort_keys=True).splitlines()
    diff = difflib.unified_diff(
        base_lines,
        generated_lines,
        fromfile=f"{label} (base)",
        tofile=f"{label} (generated)",
        lineterm="",
    )
    print(color_message(f"ERROR: runtime config for '{label}' changed after schema.codegen:", "red"))
    for line in diff:
        print(line)
    print("---")
    return False


@task
def check_codegen_drift(ctx, output_dir=None):
    """
    Verify that regenerating the config settings code does not change the
    Agent's runtime configuration defaults.

    Steps:
      1. Build the Agent and dump the core/system-probe config defaults (base).
      2. Run `schema.codegen --fix --keep-orig-order`.
      3. Rebuild the Agent and dump the config defaults again (generated).
      4. Diff base vs generated and fail on any difference.

    output_dir: directory where the base/generated JSON dumps are written. A
                temporary directory is used when omitted. In CI, pass a path
                inside the project so the dumps can be collected as artifacts.
    """
    if output_dir is None:
        output_dir = tempfile.mkdtemp(prefix="config-codegen-drift-")
    os.makedirs(output_dir, exist_ok=True)

    agent_bin = _agent_bin()

    # Step 1: build the Agent and dump the "base" configuration.
    print("Building the Agent (base)...")
    ctx.run("dda inv -- agent.build")
    for target in TARGETS:
        _dump_config(ctx, agent_bin, target, os.path.join(output_dir, f"base_{target}.json"))

    # Step 2: regenerate the config settings code.
    print("Regenerating config settings code (schema.codegen --fix --keep-orig-order)...")
    ctx.run("dda inv -- schema.codegen --fix --keep-orig-order")

    # Step 3: rebuild the Agent and dump the "generated" configuration.
    print("Rebuilding the Agent (generated)...")
    ctx.run("dda inv -- agent.build")
    for target in TARGETS:
        _dump_config(ctx, agent_bin, target, os.path.join(output_dir, f"generated_{target}.json"))

    # Step 4: diff base vs generated for every target.
    ok = True
    for target in TARGETS:
        base = os.path.join(output_dir, f"base_{target}.json")
        generated = os.path.join(output_dir, f"generated_{target}.json")
        if not _diff_config(base, generated, target):
            ok = False

    if not ok:
        raise Exit(
            color_message(
                "Regenerating the config settings code changed the Agent's runtime configuration.\n"
                "This means the generated code (`dda inv schema.codegen`) is not equivalent to the\n"
                "hand-written settings in pkg/config/setup/. Inspect the diff above and the changes\n"
                "the codegen made to pkg/config/setup/.",
                "red",
            ),
            code=1,
        )

    print(color_message("[Success] schema.codegen does not change the runtime configuration.", "green"))
