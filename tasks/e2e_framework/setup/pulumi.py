import os
import shutil
from pathlib import Path

from invoke.context import Context
from invoke.exceptions import UnexpectedExit

from tasks.e2e_framework.config import Config
from tasks.e2e_framework.tool import ask, info, is_linux, is_windows, warn


def setup_pulumi_config(config):
    if config.configParams.pulumi is None:
        config.configParams.pulumi = Config.Params.Pulumi(
            logLevel=1,
            logToStdErr=False,
        )
    # log level
    if config.configParams.pulumi.logLevel is None:
        config.configParams.pulumi.logLevel = 1
    default_log_level = config.configParams.pulumi.logLevel
    info(
        "Pulumi emits logs at log levels between 1 and 11, with 11 being the most verbose. At log level 10 or below, Pulumi will avoid intentionally exposing any known credentials. At log level 11, Pulumi will intentionally expose some known credentials to aid with debugging, so these log levels should be used only when absolutely needed."
    )
    while True:
        log_level = ask(f"🔊 Pulumi log level (1-11) - empty defaults to [{default_log_level}]: ")
        if len(log_level) == 0:
            config.configParams.pulumi.logLevel = default_log_level
            break
        if log_level.isdigit() and 1 <= int(log_level) <= 11:
            config.configParams.pulumi.logLevel = int(log_level)
            break
        warn(f"Expecting log level between 1 and 11, got {log_level}")
    # APP key
    if config.configParams.pulumi.logToStdErr is None:
        config.configParams.pulumi.logToStdErr = False
    default_logs_to_std_err = config.configParams.pulumi.logToStdErr
    while True:
        logs_to_std_err = ask(f"Write pulumi logs to stderr - empty defaults to [{default_logs_to_std_err}]: ")
        if len(logs_to_std_err) == 0:
            config.configParams.pulumi.logToStdErr = default_logs_to_std_err
            break
        if logs_to_std_err.lower() in ["true", "false"]:
            config.configParams.pulumi.logToStdErr = logs_to_std_err.lower() == "true"
            break
        warn(f"Expecting one of [true, false], got {logs_to_std_err}")


def pulumi_version(ctx: Context) -> tuple[str, bool]:
    """
    Returns True if pulumi is installed and up to date, False otherwise
    Will return True if PULUMI_SKIP_UPDATE_CHECK=1
    """
    try:
        out = ctx.run("pulumi version --logtostderr", hide=True)
    except UnexpectedExit:
        # likely pulumi command not found
        return "", False
    if out is None:
        return "", False
    # The update message differs some between platforms so choose a common part
    up_to_date = "A new version of Pulumi is available" not in out.stderr
    return out.stdout.strip(), up_to_date


def install_pulumi(ctx: Context):
    info("🤖 Install Pulumi")
    if is_windows():
        ctx.run("winget install pulumi")
    elif is_linux():
        ctx.run("curl -fsSL https://get.pulumi.com | sh")
    else:
        ctx.run("brew install pulumi/tap/pulumi")
    # If pulumi was just installed for the first time it's probably not on the PATH,
    # add it to the process env so rest of setup can continue.
    if shutil.which("pulumi") is None:
        print()
        warn("Pulumi is not in the PATH, please add pulumi to PATH before running tests")
        if is_windows():
            # Add common pulumi install locations to PATH
            paths = [
                str(x)
                for x in [
                    Path().home().joinpath(".pulumi", "bin"),
                    Path().home().joinpath("AppData", "Local", "pulumi", "bin"),
                    'C:\\Program Files (x86)\\Pulumi\\bin',
                    'C:\\Program Files (x86)\\Pulumi',
                ]
            ]
            os.environ["PATH"] = ';'.join([os.environ["PATH"]] + paths)
        elif is_linux():
            path = Path().home().joinpath(".pulumi", "bin")
            os.environ["PATH"] = f"{os.environ['PATH']}:{path}"
