import os
import secrets
import shutil
from pathlib import Path

from invoke.context import Context
from invoke.exceptions import UnexpectedExit

from tasks.e2e_framework.config import Config
from tasks.e2e_framework.tool import info, is_linux, is_windows, warn

# Length matches Pulumi cloud-generated passphrases. token_urlsafe yields ~43 chars from 32 bytes.
_PASSPHRASE_BYTES = 32


def setup_pulumi_config(config: Config):
    """
    Apply silent defaults for the Pulumi config block: pick a sensible log level,
    set logToStdErr on, and generate a random passphrase if none exists.

    Re-running is safe: existing values are preserved.
    """
    if config.configParams.pulumi is None:
        config.configParams.pulumi = Config.Params.Pulumi()

    pulumi = config.configParams.pulumi

    if pulumi.logLevel is None:
        pulumi.logLevel = 1
    if pulumi.logToStdErr is None:
        pulumi.logToStdErr = True

    if pulumi.verboseProgressStreams is None:
        pulumi.verboseProgressStreams = True

    if not pulumi.passphrase:
        pulumi.passphrase = secrets.token_urlsafe(_PASSPHRASE_BYTES)
        info("✓ Generated Pulumi passphrase (stored in ~/.test_infra_config.yaml, chmod 0600)")
    else:
        info("✓ Pulumi passphrase already configured")


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
