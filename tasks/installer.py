"""
installer namespaced tasks
"""

import base64
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
    go_mod="readonly",
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
            build="installer",
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
def build_linux_script(
    ctx,
    signing_key_id=None,
):
    '''
    Builds the linux script that is used to install the agent on linux.
    '''
    script_path = os.path.join(BIN_PATH, "setup.sh")
    signed_script_path = os.path.join(BIN_PATH, "setup.sh.asc")
    amd64_path = os.path.join(BIN_PATH, "bootstrapper-linux-amd64")
    arm64_path = os.path.join(BIN_PATH, "bootstrapper-linux-arm64")

    ctx.run(
        f'inv -e installer.build --bootstrapper --no-no-strip-binary --output-bin {amd64_path} --no-cgo',
        env={'GOOS': 'linux', 'GOARCH': 'amd64'},
    )
    ctx.run(
        f'inv -e installer.build --bootstrapper --no-no-strip-binary --output-bin {arm64_path} --no-cgo',
        env={'GOOS': 'linux', 'GOARCH': 'arm64'},
    )
    with open(amd64_path, 'rb') as f:
        amd64_b64 = base64.encodebytes(f.read()).decode('utf-8')
    with open(arm64_path, 'rb') as f:
        arm64_b64 = base64.encodebytes(f.read()).decode('utf-8')

    with open('pkg/fleet/installer/setup.sh') as f:
        setup_content = f.read()
    setup_content = setup_content.replace('INSTALLER_BIN_LINUX_AMD64', amd64_b64)
    setup_content = setup_content.replace('INSTALLER_BIN_LINUX_ARM64', arm64_b64)

    commit_sha = ctx.run('git rev-parse HEAD', hide=True).stdout.strip()
    setup_content = setup_content.replace('INSTALLER_COMMIT', commit_sha)

    with open(script_path, 'w') as f:
        f.write(setup_content)

    if signing_key_id:
        ctx.run(
            f'gpg --armor --batch --yes --output {signed_script_path} --clearsign --digest-algo SHA256 --default-key {signing_key_id} {script_path}',
        )
        # Add the signed footer to the setup.sh file
        with (
            open(signed_script_path) as signed_file,
            open(script_path, 'w') as f,
        ):
            skip_header = False
            for line in signed_file:
                if skip_header:
                    f.write(line)
                elif line.strip() == "":  # Empty line marks end of header
                    skip_header = True


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
