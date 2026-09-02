import contextlib
import getpass
import json
import os
import pathlib
import platform
import shlex
import shutil
import subprocess
import tempfile
from io import StringIO
from typing import Any

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.runners import Result

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


def restrict_file_to_owner(path: str | pathlib.Path) -> None:
    """
    Restrict a file holding secrets so that only its owner can read it.

    On Windows os.chmod only toggles the read-only attribute, so the ACL has to be replaced
    outright. Sandboxing and management tools grant themselves an ACE on the user profile,
    and a single inherited ACE of that kind leaves the file readable by another principal,
    which is enough for OpenSSH to reject a private key.
    """
    if not is_windows():
        os.chmod(path, 0o600)
        return
    user = os.environ.get("USERNAME") or getpass.getuser()
    # SYSTEM (S-1-5-18) and Administrators (S-1-5-32-544) are named by SID because their
    # names are localized, and granting them full control matches what ssh-keygen writes.
    subprocess.run(
        [
            "icacls",
            str(path),
            "/inheritance:r",
            "/grant:r",
            f"{user}:(F)",
            "*S-1-5-18:(F)",
            "*S-1-5-32-544:(F)",
        ],
        check=True,
        capture_output=True,
    )


def write_secret_file(path: str | pathlib.Path, content: str) -> None:
    """
    Write text to a file that only its owner can read.

    The content is staged in a file that is locked down while still empty, then moved into
    place. Writing to the target directly would expose the secret for the duration of the
    write, and truncating it would destroy the previous contents should the permission
    change then fail. These files hold the only copy of the Pulumi and SSH key passphrases.
    """
    path = pathlib.Path(path)
    # mkstemp creates the file with O_EXCL under an unpredictable name, so a symlink planted
    # at a guessable staging path cannot redirect the write and concurrent saves cannot land
    # on the same file. It shares a directory with the target because os.replace is only
    # atomic within a single filesystem.
    fd, staging = tempfile.mkstemp(dir=path.parent, prefix=f".{path.name}.", suffix=".tmp")
    try:
        # mkstemp applies mode 0600, which suffices on POSIX. Windows ignores it, so the ACL
        # is rewritten separately before any content is written.
        with os.fdopen(fd, "w") as f:
            restrict_file_to_owner(staging)
            f.write(content)
        os.replace(staging, path)
    except BaseException:
        with contextlib.suppress(OSError):
            os.remove(staging)
        raise


def ask(question: str, color: str = "blue") -> str:
    return input(colored(question, color))


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


def get_local_user() -> str:
    """
    The developer's login.

    On a shared dev VM (a Datadog workspace) the OS user is `bits` for everyone, so the
    real identity comes from $REAL_USER.
    """
    return os.getenv("REAL_USER") or getpass.getuser()


def get_resource_owner_id() -> str:
    """
    The identity used to name cloud resources: Pulumi stacks, EC2 key pairs and SSH keys.

    The workspace name is part of it so that several workspaces owned by the same
    developer don't fight over the same key pair or stack names in a shared account.
    """
    parts = [get_local_user()]
    workspace = os.getenv("WORKSPACE_NAME")
    if workspace:
        parts.append(workspace)
    # EKS doesn't support '.' and spaces in the user name could be problematic on Windows
    return "-".join(parts).replace(".", "-").replace(" ", "-").lower()


def get_stack_name(stack_name: str | None, scenario_name: str) -> str:
    if stack_name is None:
        stack_name = scenario_name.replace("/", "-")
    # The scenario name cannot start with the stack name because ECS
    # stack name cannot start with 'ecs' or 'aws'.
    # Normalizing here rather than in the caller keeps deploy and destroy looking for the
    # same name: they used to disagree on the casing of a --stack-name.
    return f"{get_stack_name_prefix()}{stack_name}".replace(" ", "-").lower()


def get_stack_name_prefix() -> str:
    return f"{get_resource_owner_id()}-"


CI_PULUMI_BACKEND_URL = "s3://dd-pulumi-state"
CI_PULUMI_PASSPHRASE_SSM_PARAM = "ci.datadog-agent.pulumi_password"


def _ci_pulumi_env() -> dict[str, str]:
    """
    Point the Pulumi CLI at CI's S3 state backend and supply the CI secrets passphrase
    (read from SSM), so a CI-created stack -- absent from the local file backend -- can be
    queried.

    Must be run with AWS credentials that can read CI_PULUMI_BACKEND_URL and decrypt
    CI_PULUMI_PASSPHRASE_SSM_PARAM, e.g.:
        aws-vault exec sso-agent-qa-read-only -- dda inv aws.rdp-vm --stack-name=<ci-stack-name> --ci
    """
    import boto3

    ssm = boto3.client("ssm", region_name="us-east-1")
    passphrase = ssm.get_parameter(Name=CI_PULUMI_PASSPHRASE_SSM_PARAM, WithDecryption=True)["Parameter"]["Value"]
    # Selecting the backend through the environment rather than `pulumi login` keeps the
    # dev machine's own login untouched: there is no global state to restore if the
    # command fails.
    return {
        "PULUMI_BACKEND_URL": CI_PULUMI_BACKEND_URL,
        "PULUMI_CONFIG_PASSPHRASE": passphrase,
    }


def _local_pulumi_passphrase(config_path: str | None) -> str | None:
    # Imported here because config imports this module.
    from . import config as e2e_config

    try:
        return e2e_config.get_pulumi_passphrase(e2e_config.get_local_config(config_path))
    except Exception:
        # A broken config file shouldn't stop `pulumi version` or `pulumi login` from
        # running. Commands that do need the passphrase still prompt, as they did before.
        return None


def pulumi_env(
    *,
    config_path: str | None = None,
    ci: bool = False,
    skip_update_check: bool = True,
    overrides: dict[str, str] | None = None,
) -> dict[str, str]:
    """
    Build the environment overrides that every Pulumi invocation needs.

    The passphrase is read from the local config file (~/.test_infra_config.yaml) unless
    the caller's environment already carries one, so the tasks work on a machine where it
    was never exported -- a Datadog workspace, for instance.
    """
    env: dict[str, str] = {}
    if skip_update_check:
        env["PULUMI_SKIP_UPDATE_CHECK"] = "true"

    # Membership rather than truthiness: an empty passphrase is a valid Pulumi setup.
    if "PULUMI_CONFIG_PASSPHRASE" not in os.environ and "PULUMI_CONFIG_PASSPHRASE_FILE" not in os.environ:
        passphrase = _local_pulumi_passphrase(config_path)
        if passphrase:
            env["PULUMI_CONFIG_PASSPHRASE"] = passphrase

    if ci:
        env.update(_ci_pulumi_env())
    if overrides:
        env.update(overrides)
    return env


def run_pulumi(
    ctx: Context,
    args: str,
    *,
    stack: str | None = None,
    project_dir: bool | str = True,
    config_path: str | None = None,
    ci: bool = False,
    env: dict[str, str] | None = None,
    skip_update_check: bool = True,
    pty: bool = False,
    hide: str | bool | None = None,
    warn: bool = False,
) -> Result | None:
    """
    Run a Pulumi command.

    Every Pulumi invocation in the e2e tasks goes through here, so the project directory,
    the passphrase and the rest of the environment are resolved in a single place.

    `project_dir` drives the -C flag: True picks the e2e-framework Pulumi project, False
    omits it (for commands that aren't project scoped, such as `login` or `version`), and
    a path uses that directory.
    """
    cmd_parts = ["pulumi"]
    if project_dir is True:
        dir_flag = get_pulumi_dir_flag()
        if dir_flag:
            cmd_parts.append(dir_flag)
    elif project_dir is not False:
        cmd_parts.append(f"-C {shlex.quote(str(project_dir))}")
    # Callers build args by interpolating optional flag groups, so it can come with
    # leading or trailing blanks when a group is empty.
    if args.strip():
        cmd_parts.append(args.strip())
    if stack is not None:
        cmd_parts.append(f"-s {stack}")

    return ctx.run(
        " ".join(cmd_parts),
        env=pulumi_env(config_path=config_path, ci=ci, skip_update_check=skip_update_check, overrides=env),
        # A pty is only ever useful for the commands whose progress the user watches, and
        # invoke can't allocate one on Windows.
        pty=pty and not is_windows(),
        hide=hide,
        warn=warn,
    )


def pulumi_json(
    ctx: Context,
    args: str,
    *,
    stack: str | None = None,
    project_dir: bool | str = True,
    config_path: str | None = None,
    ci: bool = False,
    warn: bool = False,
) -> Any:
    """
    Run a Pulumi command that prints JSON and return the parsed output, or None when the
    command failed and `warn` is set.

    stdout is captured instead of echoed; stderr keeps streaming so Pulumi's own error
    message stays visible when the command fails.
    """
    result = run_pulumi(
        ctx, args, stack=stack, project_dir=project_dir, config_path=config_path, ci=ci, hide="stdout", warn=warn
    )
    if result is None or result.exited != 0:
        return None
    return json.loads(result.stdout)


def pulumi_stack_names(
    ctx: Context,
    *,
    project: str | None = None,
    project_dir: bool | str = True,
    config_path: str | None = None,
) -> list[str]:
    """
    Names of every stack the current backend knows about.
    """
    args = "stack ls --all --json"
    if project:
        args += f" --project {project}"
    stacks = pulumi_json(ctx, args, project_dir=project_dir, config_path=config_path) or []
    return [stack["name"] for stack in stacks if "name" in stack]


def get_stack_json_outputs(ctx: Context, full_stack_name: str, config_path: str | None = None, ci: bool = False) -> Any:
    return pulumi_json(
        ctx,
        "stack output --json --show-secrets",
        stack=full_stack_name,
        config_path=config_path,
        ci=ci,
    )


def get_aws_wrapper(
    aws_account: str,
) -> str:
    return f"aws-vault exec sso-{aws_account}-account-admin-8h -- "


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


def get_image_description(ctx: Context, ami_id: str) -> Any:
    buffer = StringIO()
    ctx.run(
        f"aws-vault exec sso-agent-sandbox-account-admin-8h -- aws ec2 describe-images --image-ids {ami_id}",
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
    # Headless Linux boxes -- Datadog workspaces among them -- have no libnotify and no
    # D-Bus session, so there is nothing to notify.
    if shutil.which("notify-send") is None:
        return
    ctx.run(f"notify-send 'test/e2e-framework' '{text}'")


def notify_windows():
    # TODO: Implenent notification on windows. Would require windows computer (with desktop) to test
    return


def copy_to_clipboard_if_supported(text: str, prompt: str | None = None) -> bool:
    """
    Copy `text` to the clipboard, and return whether that worked.

    Headless machines -- Datadog workspaces among them -- have no clipboard, so the copy
    is probed before `prompt` is shown: asking the user to press a key and only then
    failing would be worse than not offering the copy at all.
    """
    try:
        import pyperclip

        pyperclip.copy("")
    except Exception:
        return False

    if prompt:
        input(prompt)
    pyperclip.copy(text)
    return True


# ensure we run pulumi from a directory with a Pulumi.yaml file
# defaults to the project root directory
def get_pulumi_dir_flag():
    root_path = get_pulumi_run_folder()
    current_path = os.getcwd()
    if not os.path.isfile(os.path.join(current_path, "Pulumi.yaml")):
        return f"-C {shlex.quote(root_path)}"
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
        copy_to_clipboard_if_supported(command, prompt="Press a key to copy command to clipboard...")


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
