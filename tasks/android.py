"""
Android namespaced tasks
"""
from __future__ import print_function
import glob
import yaml
import os
import shutil
import sys
from distutils.dir_util import copy_tree

import invoke
from invoke import task
from invoke.exceptions import Exit

from .utils import bin_name, get_build_flags, load_release_versions, get_version
from .utils import REPO_PATH
from .build_tags import get_default_build_tags
from .go import deps, generate

# constants
BIN_PATH = os.path.join(".", "bin", "agent")
AGENT_TAG = "datadog/agent:master"
from .agent import DEFAULT_BUILD_TAGS

ANDROID_CORECHECKS = [
    "cpu",
    "disk",
    "io",
    "load",
    "memory",
    "network",
    "uptime",
]
CORECHECK_CONFS_DIR = "cmd/agent/android/app/src/main/assets/conf.d"
@task
def build(ctx, rebuild=False, race=False, build_include=None, build_exclude=None,
        development=True, precompile_only=False, skip_assets=False, major_version='7',
        python_runtimes='3'):
    """
    Build the android apk. If the bits to include in the build are not specified,
    the values from `invoke.yaml` will be used.

    Example invokation:
        inv android.build
    """
    # ensure BIN_PATH exists
    if not os.path.exists(BIN_PATH):
        os.makedirs(BIN_PATH)

    # put the check confs in place
    assetconfigs(ctx)

    ldflags, gcflags, env = get_build_flags(ctx, major_version=major_version, python_runtimes=python_runtimes)

    # Generating go source from templates by running go generate on ./pkg/status
    generate(ctx)

    build_tags = get_default_build_tags(android=True)

    build_tags.append("android")
    cmd = "gomobile bind -target android {race_opt} {build_type} -tags \"{go_build_tags}\" "

    cmd += "-o {agent_bin} -gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/agent/android"
    args = {
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else "",
        "go_build_tags": " ".join(build_tags),
        "agent_bin": os.path.join(BIN_PATH, bin_name("ddagent", android=True)),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }
    # gomobile is not supporting go modules
    # https://go-review.googlesource.com/c/mobile/+/167659/
    env["GO111MODULE"] = "off"
    ctx.run(cmd.format(**args), env=env)

    pwd = os.getcwd()
    os.chdir("cmd/agent/android")
    if sys.platform == 'win32':
        cmd = "gradlew.bat --no-daemon build"
    else:
        cmd = "./gradlew --no-daemon build"
    ctx.run(cmd)
    os.chdir(pwd)
    ver = get_version(ctx, include_git=True, git_sha_length=7, major_version=major_version)
    outfile = "bin/agent/ddagent-{}-unsigned.apk".format(ver)
    shutil.copyfile("cmd/agent/android/app/build/outputs/apk/release/app-release-unsigned.apk", outfile)


@task
def sign_apk(ctx, development=True):
    """
    Signs the APK with the default platform signature.
    """
    cmd = "java -jar signapk.jar platform.x509.pem platform.pk8 bin/agent/ddagent-release-unsigned.apk bin/agent/ddagent-release-signed.apk"
    ctx.run(cmd)


@task
def install(ctx, rebuild=False, race=False, build_include=None, build_exclude=None,
        skip_build=False):
    """
    Installs the APK on a device.

    By default it builds the agent before executing it, unless --skip-build was
    passed. It accepts the same set of options as agent.build.
    """
    if not skip_build:
        build(ctx, rebuild, race, build_include, build_exclude)

    sign_apk(ctx)
    cmd = "adb install -r bin/agent/ddagent-release-signed.apk"
    ctx.run(cmd)


@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/agent folder
    print("Remove agent binary folder")
    shutil.rmtree("./bin/agent")

    shutil.rmtree(CORECHECK_CONFS_DIR)

@task
def assetconfigs(ctx):
    # move the core check config
    try:
        shutil.rmtree(CORECHECK_CONFS_DIR)
    except:
        ## it's ok if the dir is not there
        pass

    files = {}
    files_list = []
    os.makedirs(CORECHECK_CONFS_DIR)
    for check in ANDROID_CORECHECKS:
        srcfile = "cmd/agent/dist/conf.d/{}.d/conf.yaml.default".format(check)
        tgtfile = "{}/{}.yaml".format(CORECHECK_CONFS_DIR, check)
        shutil.copyfile(srcfile, tgtfile)
        files_list.append("{}.yaml".format(check))
    files["files"] = files_list

    with open("{}/directory_manifest.yaml".format(CORECHECK_CONFS_DIR), 'w') as outfile:
        yaml.dump(files, outfile, default_flow_style=False)

@task
def launchservice(ctx, api_key, hostname=None, tags=None):
    if api_key is None:
        print("must supply api key")
        return

    if hostname is None:
        print("Setting hostname to android-tablet")
        hostname="android-tablet"

    if tags is None:
        print("Setting tags to owner:db,env:local,role:windows")
        tags="owner:db,env:local,role:windows"

    cmd = "adb shell am startservice --es api_key {} --es hostname {} --es tags {} org.datadog.agent/.DDService".format(api_key, hostname, tags)
    ctx.run(cmd)

@task
def stopservice(ctx):
    cmd = "adb shell am force-stop org.datadog.agent"
    ctx.run(cmd)
