"""
installer namespaced tasks
"""

import glob
import hashlib
from os import makedirs, path

from invoke import task

from tasks.build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags

DIR_BIN = path.join(".", "bin", "installer")
INSTALLER_BIN = path.join(DIR_BIN, bin_name("installer"))
INSTALL_SCRIPT_TEMPLATE = path.join("pkg", "fleet", "installer", "setup", "install.sh")

MAJOR_VERSION = '7'


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
):
    """
    Build the installer.
    """

    ldflags, gcflags, env = get_build_flags(
        ctx, major_version=MAJOR_VERSION, install_path=install_path, run_path=run_path
    )

    build_include = (
        get_default_build_tags(
            build="installer",
        )  # TODO/FIXME: Arch not passed to preserve build tags. Should this be fixed?
        if build_include is None
        else filter_incompatible_tags(build_include.split(","))
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    build_tags = get_build_tags(build_include, build_exclude)

    strip_flags = "" if no_strip_binary else "-s -w"
    race_opt = "-race" if race else ""
    build_type = "-a" if rebuild else ""
    go_build_tags = " ".join(build_tags)

    installer_bin = INSTALLER_BIN
    if output_bin:
        installer_bin = output_bin

    if no_cgo:
        env["CGO_ENABLED"] = "0"
    else:
        env["CGO_ENABLED"] = "1"

    cmd = f"go build -mod={go_mod} {race_opt} {build_type} -tags \"{go_build_tags}\" "
    cmd += f"-o {installer_bin} -gcflags=\"{gcflags}\" -ldflags=\"{ldflags} {strip_flags}\" {REPO_PATH}/cmd/installer"

    ctx.run(cmd, env=env)


@task
def build_linux_script(ctx, flavor, version, bin_amd64, bin_arm64, output):
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
        if "Before=" in line:
            continue  # Skip line
        if "After=" in line:
            after_no_agent = line.split("After=")[1].replace("datadog-agent.service", "").strip()
            if len(after_no_agent) == 0:
                continue  # Skip line
            line = "After=" + after_no_agent + "\n"
        if "BindsTo=" in line:
            line = line.replace(".service", "-exp.service")

        if "Alias=" in line:
            line = line.replace(".service", "-exp.service")
        if "Description=" in line:
            line = line.replace("\n", "") + " Experiment\n"
        line = line.replace("stable", "experiment")
        experiment_file += line
    return experiment_file


def generate_core_agent_experiment_unit(f):
    """
    Generates the core agent experiment unit file.
    """

    experiment_file = ""
    for line in f:
        if "After=" in line:
            line += "OnFailure=datadog-agent.service\n"
            line += "JobTimeoutSec=3000\n"
        if "Wants=" in line:
            line = line.replace(".service", "-exp.service")
        if "Type=" in line:
            line = "Type=oneshot\n"
        if "Conflicts=" in line:
            line = "Conflicts=datadog-agent.service\n"
        if "Before=" in line:
            line = "Before=datadog-agent.service\n"
        if "Restart=" in line or "# " in line or "StartLimitInterval=" in line or "StartLimitBurst=" in line:
            continue  # Skip line
        if "ExecStart=" in line:
            line += "ExecStart=/bin/false\n"
            line += "ExecStop=/bin/false\n"

        if "Alias=" in line:
            line = line.replace(".service", "-exp.service")
        if "Description=" in line:
            line = line.replace("\n", "") + " Experiment\n"
        line = line.replace("stable", "experiment")
        experiment_file += line
    return experiment_file
