from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task


@task(
    help={
        "stack_name": "Name of stack that contains hosts to fetch the password for",
        "ip": "Filter to VM with this IP address",
        "ci": "Look up a CI-created stack (state in CI's S3 backend) instead of a local one. "
        "Run wrapped in `aws-vault exec sso-agent-qa-read-only --`.",
        "config_path": "Path to the local config file holding the Pulumi passphrase "
        "(defaults to ~/.test_infra_config.yaml). Ignored when --ci is set.",
    }
)
def get_vm_password(
    ctx: Context,
    stack_name: str | None = None,
    ip: str | None = None,
    ci: bool = False,
    config_path: str | None = None,
):
    """
    Print the password of virtual machines in a stack.

    Cloud-agnostic: reads the password straight from the stack's Pulumi
    outputs (the same value RemoteHost.Password uses in-process), so it
    works for any provisioner that exports a password (AWS, Azure, ...).
    """
    from tasks.e2e_framework import config, tool

    if not stack_name:
        raise Exit("Please provide a stack name to connect to.")

    passphrase_ctx = tool.use_ci_pulumi_backend(ctx) if ci else config.use_local_pulumi_passphrase(config_path)
    with passphrase_ctx:
        out = tool.get_stack_json_outputs(ctx, stack_name)
    if not out:
        raise Exit("No VM found in the stack.")

    found = False
    for vm_id, vm in out.items():
        if "address" not in vm or not vm.get("password"):
            continue
        if ip and vm["address"] != ip:
            continue
        found = True
        print(f"Password for VM {vm_id} ({vm['address']}): {vm['password']}")

    if not found:
        raise Exit("No VM with a password found in the stack.")
