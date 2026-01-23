import getpass
import json
import os
import pathlib
import platform
from io import StringIO
from typing import Any

from invoke.context import Context
from invoke.exceptions import Exit

try:
    from termcolor import colored
except ImportError:

    def colored(*args):  # type: ignore
        return args[0]


def is_windows():
    return platform.system() == "Windows"


if is_windows():
    try:
        # Explicitly enable terminal colors work on Windows
        # os.system() seems to implicitly enable them, but ctx.run() does not
        from colorama import just_fix_windows_console

        just_fix_windows_console()
    except ImportError:
        print(
            "colorama is not up to date, terminal colors may not work properly. Please run 'dda self dep sync' to fix this."
        )


def ask(question: str) -> str:
    return input(colored(question, "blue"))


def ask_yesno(question: str, default='N') -> bool:
    res = ""
    yes_opts = ["y", "yes"]
    no_opts = ["n", "no"]
    while res.lower() not in (yes_opts + no_opts):
        res = ask(question + f" [Y/N] Default [{default}]: ")
        if res == "":
            res = default
            break

    return res.lower() in yes_opts


def debug(msg: str):
    print(colored(msg, "white"))


def info(msg: str):
    print(colored(msg, "green"))


def warn(msg: str):
    print(colored(msg, "yellow"))


def error(msg: str):
    print(colored(msg, "red"))


def get_default_agent_install() -> bool:
    return True


def get_default_agent_with_operator_install() -> bool:
    return False


def get_default_workload_install() -> bool:
    return True


def get_stack_name(stack_name: str | None, scenario_name: str) -> str:
    if stack_name is None:
        stack_name = scenario_name.replace("/", "-")
    # The scenario name cannot start with the stack name because ECS
    # stack name cannot start with 'ecs' or 'aws'
    return f"{get_stack_name_prefix()}{stack_name}"


def get_stack_name_prefix() -> str:
    user_name = f"{getpass.getuser()}-"
    # EKS doesn't support '.' and spaces in the user name could be problematic on Windows
    return user_name.replace(".", "-").replace(" ", "-")


def get_stack_json_outputs(ctx: Context, full_stack_name: str) -> Any:
    buffer = StringIO()

    cmd_parts: list[str] = [
        "pulumi",
        "stack",
        "output",
        "--json",
        "--show-secrets",
        "-s",
        full_stack_name,
        get_pulumi_dir_flag(),
    ]
    ctx.run(
        " ".join(cmd_parts),
        out_stream=buffer,
    )
    return json.loads(buffer.getvalue())


def get_stack_json_resources(ctx: Context, full_stack_name: str) -> Any:
    buffer = StringIO()
    with ctx.cd(_get_root_path()):
        ctx.run(
            f"pulumi stack export -s {full_stack_name}",
            out_stream=buffer,
        )
    out = json.loads(buffer.getvalue())
    return out['deployment']['resources']


def get_aws_wrapper(
    aws_account: str,
) -> str:
    return f"aws-vault exec sso-{aws_account}-account-admin -- "


def get_aws_cmd(
    cmd: str,
    use_aws_vault: bool | None = True,
    aws_account: str | None = None,
) -> str:
    wrapper = ""
    if use_aws_vault:
        if aws_account is None:
            raise Exit("AWS account is required when using aws-vault.")
        wrapper = get_aws_wrapper(aws_account)
    # specify .exe for windows to work around conflicts with aws.rb
    aws = "aws.exe" if is_windows() else "aws"
    cmd = f"{wrapper}{aws} {cmd}"
    return cmd


def is_linux():
    return platform.system() == "Linux"


def is_wsl():
    return "microsoft" in platform.uname().release.lower()


def get_aws_instance_password_data(
    ctx: Context, vm_id: str, key_path: str, aws_account: str | None = None, use_aws_vault: bool | None = True
) -> str:
    buffer = StringIO()
    with ctx.cd(_get_root_path()):
        cmd = f'aws ec2 get-password-data --instance-id "{vm_id}" --priv-launch-key "{key_path}"'
        if use_aws_vault:
            if aws_account is None:
                raise Exit("AWS account is required when using aws-vault.")
            cmd = get_aws_wrapper(aws_account) + cmd
        ctx.run(cmd, out_stream=buffer)
    out = json.loads(buffer.getvalue())
    return out["PasswordData"]


def get_image_description(ctx: Context, ami_id: str) -> Any:
    buffer = StringIO()
    ctx.run(
        f"aws-vault exec sso-agent-sandbox-account-admin -- aws ec2 describe-images --image-ids {ami_id}",
        out_stream=buffer,
    )
    result = json.loads(buffer.getvalue())
    if len(result["Images"]) > 1:
        raise Exit(f"The AMI id {ami_id} returns more than one definition.")
    else:
        return result["Images"][0]


def rdp(ctx, ip):
    if is_windows() or is_wsl():
        rdp_windows(ctx, ip)
    elif is_linux():
        raise Exit("RDP is not yet implemented on Linux")
    else:
        rdp_macos(ctx, ip)


def rdp_windows(ctx, ip):
    ctx.run(f"mstsc.exe /v:{ip}", disown=True)


def rdp_macos(ctx, ip):
    ctx.run(f"open -a '/Applications/Microsoft Remote Desktop.app' rdp://{ip}", disown=True)


def notify(ctx, text):
    if is_linux():
        notify_linux(ctx, text)
    elif is_windows():
        notify_windows()
    else:
        notify_macos(ctx, text)


def notify_macos(ctx, text):
    CMD = '''
    on run argv
    display notification (item 2 of argv) with title (item 1 of argv)
    end run
    '''
    ctx.run(f"osascript -e '{CMD}' test/e2e-framework '{text}'")


def notify_linux(ctx, text):
    ctx.run(f"notify-send 'test/e2e-framework' '{text}'")


def notify_windows():
    # TODO: Implenent notification on windows. Would require windows computer (with desktop) to test
    return


# ensure we run pulumi from a directory with a Pulumi.yaml file
# defaults to the project root directory
def get_pulumi_dir_flag():
    root_path = get_pulumi_run_folder()
    current_path = os.getcwd()
    if not os.path.isfile(os.path.join(current_path, "Pulumi.yaml")):
        return f"-C {root_path}"
    return ""


def _get_root_path() -> str:
    folder = pathlib.Path(__file__).parent.parent.resolve()
    return str(folder.parent)


def get_pulumi_run_folder() -> str:
    return os.path.join(_get_root_path(), "test", "e2e-framework", "run")


class RemoteHost:
    def __init__(self, name, stack_outputs: Any):
        remoteHost: Any = stack_outputs[f"dd-Host-{name}"]
        self.address: str = remoteHost["address"]
        self.user: str = remoteHost["username"]
        self.password: str | None = "password" in remoteHost and remoteHost["password"] or None
        self.port: int | None = "port" in remoteHost and remoteHost["port"] or None


def show_connection_message(
    ctx: Context, remote_host_name: str, full_stack_name: str, copy_to_clipboard: bool | None = True
):
    outputs = get_stack_json_outputs(ctx, full_stack_name)
    remoteHost = RemoteHost(remote_host_name, outputs)
    address = remoteHost.address
    user = remoteHost.user

    command = f"ssh {user}@{address}"

    if remoteHost.port:
        command += f" -p {remoteHost.port}"

    print(f"\nYou can run the following command to connect to the host `{command}`.\n")
    if copy_to_clipboard:
        import pyperclip

        input("Press a key to copy command to clipboard...")
        pyperclip.copy(command)


def add_known_host(ctx: Context, address: str) -> None:
    """
    Add the host to the known_hosts file.
    """
    # remove the host if it already exists
    clean_known_hosts(ctx, address)
    result = ctx.run(f"ssh-keyscan {address}", hide=True, warn=True)
    if result and result.ok:
        home = pathlib.Path.home()
        filtered_hosts = '\n'.join([line for line in result.stdout.splitlines() if not line.startswith("#")])
        with open(os.path.join(home, ".ssh", "known_hosts"), "a") as f:
            f.write(filtered_hosts)


def clean_known_hosts(ctx: Context, host: str) -> None:
    """
    Remove the host from the known_hosts file.
    """
    ctx.run(f"ssh-keygen -R {host}", hide=True)


def get_host(ctx: Context, remote_host_name: str, scenario_name: str, stack_name: str | None = None) -> RemoteHost:
    """
    Get the host of the VM.
    """
    full_stack_name = get_stack_name(stack_name, scenario_name)
    outputs = get_stack_json_outputs(ctx, full_stack_name)
    return RemoteHost(remote_host_name, outputs)
