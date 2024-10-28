"""
installer namespaced tasks
"""

import os
import shutil

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags
from tasks.libs.releasing.version import get_version

BIN_PATH = os.path.join(".", "bin", "installer")
MAJOR_VERSION = '7'


@task
def build(
    ctx,
    output_bin=None,
    bootstrapper=False,
    rebuild=False,
    race=False,
    install_path=None,
    run_path=None,
    build_include=None,
    build_exclude=None,
    go_mod="mod",
    no_strip_binary=True,
    no_cgo=False,
):
    """
    Build the updater.
    """

    ldflags, gcflags, env = get_build_flags(
        ctx, major_version=MAJOR_VERSION, install_path=install_path, run_path=run_path
    )

    build_include = (
        get_default_build_tags(
            build="updater",
        )  # TODO/FIXME: Arch not passed to preserve build tags. Should this be fixed?
        if build_include is None
        else filter_incompatible_tags(build_include.split(","))
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    build_tags = get_build_tags(build_include, build_exclude)
    if bootstrapper:
        build_tags.append("bootstrapper")

    strip_flags = "" if no_strip_binary else "-s -w"
    race_opt = "-race" if race else ""
    build_type = "-a" if rebuild else ""
    go_build_tags = " ".join(build_tags)

    installer_bin_name = "installer"
    if bootstrapper:
        installer_bin_name = "bootstrapper"
    installer_bin = os.path.join(BIN_PATH, bin_name(installer_bin_name))
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
def push_artifact(
    ctx,
    artifact,
    registry,
    version="",
    tag="latest",
    arch="amd64",
):
    '''
    Pushes an OCI artifact to a registry.
    example:
        inv -e installer.push-artifact --artifact "datadog-installer" --registry "docker.io/myregistry" --tag "latest"
    '''
    if version == "":
        version = get_version(ctx, include_git=True, url_safe=True, major_version='7', include_pipeline_id=True)

    # structural pattern matching is only available in Python 3.10+, which currently fails the `vulture` check
    if artifact == 'datadog-agent':
        image_name = 'agent-package'
    elif artifact == 'datadog-installer':
        image_name = 'installer-package'
    else:
        print("Unexpected artifact")
        raise Exit(code=1)

    if os.name == 'nt':
        target_os = 'windows'
    else:
        print('Unexpected os')
        raise Exit(code=1)

    datadog_package = shutil.which('datadog-package')
    if datadog_package is None:
        print('datadog-package could not be found in path')
        raise Exit(code=1)

    ctx.run(
        f'{datadog_package} push {registry}/{image_name}:{tag} omnibus/pkg/{artifact}-{version}-1-{target_os}-{arch}.oci.tar'
    )
