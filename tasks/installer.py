"""
installer namespaced tasks
"""

import glob
import hashlib
import sys
from os import getenv, makedirs, path

from invoke import task

from tasks.build_tags import (
    compute_build_tags_for_flavor,
)
from tasks.flavor import AgentFlavor
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags
from tasks.windows_resources import build_messagetable, build_rc, versioninfo_vars

DIR_BIN = path.join(".", "bin", "installer")
INSTALLER_BIN = path.join(DIR_BIN, bin_name("installer"))
INSTALL_SCRIPT_TEMPLATE = path.join("pkg", "fleet", "installer", "setup", "install.sh")


@task
def build(
    ctx,
    output_bin=None,
    rebuild=False,
    race=False,
    install_path=None,
    run_path=None,
    build_include=None,
    build_exclude=None,
    go_mod="readonly",
    no_strip_binary=True,
    no_cgo=False,
    fips_mode=False,
):
    """
    Build the installer.
    """

    ldflags, gcflags, env = get_build_flags(ctx, install_path=install_path, run_path=run_path)

    if sys.platform == 'win32':
        build_messagetable(ctx)
        vars = versioninfo_vars(ctx)
        build_rc(
            ctx,
            "cmd/installer/windows_resources/datadog-installer.rc",
            vars=vars,
            out="cmd/installer/rsrc.syso",
        )

    build_tags = compute_build_tags_for_flavor(
        build="installer",
        build_include=build_include,
        build_exclude=build_exclude,
        flavor=AgentFlavor.fips if fips_mode else AgentFlavor.base,
    )

    installer_bin = INSTALLER_BIN
    if output_bin:
        installer_bin = output_bin

    if no_cgo and not fips_mode:
        env["CGO_ENABLED"] = "0"
    else:
        env["CGO_ENABLED"] = "1"

    if not no_strip_binary:
        ldflags += " -s -w"

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/installer",
        mod=go_mod,
        race=race,
        rebuild=rebuild,
        gcflags=gcflags,
        ldflags=ldflags,
        build_tags=build_tags,
        bin_path=installer_bin,
        check_deadcode=getenv("DEPLOY_AGENT") == "true",
        env=env,
    )


@task
def build_linux_script(ctx, flavor, version, bin_amd64, bin_arm64, output, package="installer-package"):
    '''
    Builds the script that is used to install datadog on linux.
    '''

    with open(INSTALL_SCRIPT_TEMPLATE) as f:
        install_script = f.read()

    commit_sha = ctx.run('git rev-parse HEAD', hide=True).stdout.strip()
    install_script = install_script.replace('INSTALLER_COMMIT', commit_sha)
    install_script = install_script.replace('INSTALLER_FLAVOR', flavor)
    install_script = install_script.replace('INSTALLER_VERSION', version)

    bin_amd64_sha256 = hashlib.sha256(open(bin_amd64, 'rb').read()).hexdigest()
    bin_arm64_sha256 = hashlib.sha256(open(bin_arm64, 'rb').read()).hexdigest()
    install_script = install_script.replace('INSTALLER_AMD64_SHA256', bin_amd64_sha256)
    install_script = install_script.replace('INSTALLER_ARM64_SHA256', bin_arm64_sha256)
    install_script = install_script.replace('PACKAGE_NAME', package)

    if flavor in ("emr", "databricks", "dataproc"):
        install_script = install_script.replace(
            'DATADOG_AGENT_OPTIONAL_REMOVE_CMD',
            """
if command -v dpkg >/dev/null && dpkg -s datadog-agent >/dev/null 2>&1; then
  "${sudo_cmd[@]+"${sudo_cmd[@]}"}" datadog-agent purge >/dev/null 2>&1 || true
  "${sudo_cmd[@]+"${sudo_cmd[@]}"}" dpkg --purge datadog-agent >/dev/null 2>&1 || true
  DATADOG_AGENT_OPTIONAL_REMOVE_DEB_CMD
elif command -v rpm >/dev/null && rpm -q datadog-agent >/dev/null 2>&1; then
  "${sudo_cmd[@]+"${sudo_cmd[@]}"}" datadog-agent purge >/dev/null 2>&1 || true
  "${sudo_cmd[@]+"${sudo_cmd[@]}"}" rpm -e datadog-agent >/dev/null 2>&1 || true
fi
""",
        )
    else:
        install_script = install_script.replace('DATADOG_AGENT_OPTIONAL_REMOVE_CMD', '')

    makedirs(DIR_BIN, exist_ok=True)
    with open(path.join(DIR_BIN, output), 'w') as f:
        f.write(install_script)


@task
def generate_experiment_units(ctx, check=False):
    '''
    Generates systemd units for the experiment service.
    '''

    # Get paths to all stable service files (not the generated experiment ones)
    stable_paths = [
        f
        for f in glob.glob('./pkg/fleet/installer/packages/embedded/*.service')
        if not f.endswith('-exp.service') and 'datadog-installer' not in f
    ]
    for stable_path in stable_paths:
        experiment_path = stable_path.replace(".service", "-exp.service")
        experiment_file = ""
        with open(stable_path) as f:
            # Special handling for datadog-agent.service, which is the main service
            if "datadog-agent.service" in stable_path:
                experiment_file = generate_core_agent_experiment_unit(f)
            else:
                experiment_file = generate_subprocess_experiment_unit(f)

        if not check:
            with open(experiment_path, 'w') as f:
                f.write(experiment_file)
        else:
            try:
                with open(experiment_path) as f:
                    if f.read() != experiment_file:
                        raise Exception(
                            f"File {experiment_path} is not up to date, please run `dda inv -e installer.generate-experiment-units`"
                        )
            except FileNotFoundError:
                raise Exception(
                    f"File {experiment_path} does not exist but is expected to, please run `dda inv -e installer.generate-experiment-units`"
                ) from None


def generate_subprocess_experiment_unit(f):
    """
    Generates subprocesses experiment unit file.
    """

    experiment_file = ""
    for line in f:
        if "BindsTo=" in line:
            line = line.replace(".service", "-exp.service")
        if "Conflicts=" in line:
            line = line.replace("-exp.service", ".service")
        if "Description=" in line:
            line = line.replace("\n", "") + " Experiment\n"
        line = line.replace("stable", "experiment")
        experiment_file += line
    return experiment_file


def generate_core_agent_experiment_unit(f):
    """
    Generates the core agent experiment unit file.
    """
    experiment_timeout = "3000s"
    experiment_kill_timeout = "15s"

    experiment_file = ""
    for line in f:
        if "Wants=" in line:
            line = line.replace(".service", "-exp.service")
            line += "OnFailure=datadog-agent.service\n"
            line += "Before=datadog-agent.service\n"
        if line == "[Install]\n" or "WantedBy=" in line:
            continue  # Skip line
        if "Restart=" in line:
            line = "Restart=no\n"
        if "Description=" in line:
            line = line.replace("\n", "") + " Experiment\n"
        if "Conflicts=" in line:
            line = "Conflicts=datadog-agent.service\n"
        if "ExecStart=" in line:
            line = f"ExecStart=/usr/bin/timeout --kill-after={experiment_kill_timeout} {experiment_timeout} {line.replace('ExecStart=', '')[:-1]}\nExecStopPost=/bin/false\n"
        line = line.replace("stable", "experiment")
        experiment_file += line

    # Remove additional trailing new lines
    experiment_file = experiment_file.rstrip("\n") + "\n"

    return experiment_file
