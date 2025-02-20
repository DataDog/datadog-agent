"""
installer namespaced tasks
"""

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
