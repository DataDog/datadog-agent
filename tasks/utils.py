"""
Miscellaneous functions, no tasks here
"""
from __future__ import print_function

import os
import platform
import re
import sys
import json
from subprocess import check_output

import invoke


# constants
ORG_PATH = "github.com/DataDog"
REPO_PATH = "{}/datadog-agent".format(ORG_PATH)

def bin_name(name, android=False):
    """
    Generate platform dependent names for binaries
    """
    if android:
        return "{}.aar".format(name)

    if sys.platform == 'win32':
        return "{}.exe".format(name)
    return name


def pkg_config_path(use_embedded_libs):
    """
    Prepend the full path to either the `system` or `embedded` pkg-config
    folders provided by the agent to the existing value of `PKG_CONFIG_PATH`
    environment var.
    """
    retval = ""

    base = os.path.join(os.path.dirname("."), "pkg-config", platform.system().lower())
    if use_embedded_libs:
        retval = os.path.abspath(os.path.join(base, "embedded"))
    else:
        retval = os.path.abspath(os.path.join(base, "system"))

    # append the system wide value of PKG_CONFIG_PATH
    retval += "{}{}".format(os.pathsep, os.environ.get("PKG_CONFIG_PATH", ""))

    return retval


def get_build_flags(ctx, static=False, use_embedded_libs=False):
    """
    Build the common value for both ldflags and gcflags, and return an env accordingly.

    We need to invoke external processes here so this function need the
    Context object.
    """
    gcflags = ""
    ldflags = get_version_ldflags(ctx)
    env = {
        "PKG_CONFIG_PATH": pkg_config_path(use_embedded_libs),
        "CGO_CFLAGS_ALLOW": "-static-libgcc",  # whitelist additional flags, here a flag used for net-snmp
    }

    if sys.platform == 'win32':
        env["CGO_LDFLAGS_ALLOW"] = "-Wl,--allow-multiple-definition"

    if static:
        ldflags += "-s -w -linkmode=external '-extldflags=-static' "
        #env["CGO_ENABLED"] = "0"
    elif use_embedded_libs:
        embedded_lib_path = ctx.run("pkg-config --variable=libdir python-2.7",
                                    env=env, hide=True).stdout.strip()
        embedded_prefix = ctx.run("pkg-config --variable=prefix python-2.7",
                                  env=env, hide=True).stdout.strip()
        ldflags += "-X {}/pkg/collector/py.pythonHome={} ".format(REPO_PATH, embedded_prefix)
        ldflags += "-r {} ".format(embedded_lib_path)

    if os.environ.get("DELVE"):
        gcflags = "-N -l"
        if sys.platform == 'win32':
            # On windows, need to build with the extra argument -ldflags="-linkmode internal"
            # if you want to be able to use the delve debugger.
            ldflags += "-linkmode internal "

    return ldflags, gcflags, env


def get_payload_version():
    """
    Return the Agent payload version found in the Gopkg.toml file.
    """
    current = {}

    # parse the TOML file line by line
    with open("Gopkg.lock") as toml:
        for line in toml.readlines():
            # skip empty lines and comments
            if not line or line[0] == "#":
                continue

            # change the parser "state" when we find a [[projects]] section
            if "[[projects]]" in line:
                # see if the current section is what we're searching for
                if current.get("name") == "github.com/DataDog/agent-payload":
                    return current.get("version")

                # if not, reset the "state" and proceed with the next line
                current = {}
                continue

            # search for an assignment, ignore subsequent `=` chars
            toks = line.split('=', 2)
            if len(toks) == 2:
                # strip whitespaces
                key = toks[0].strip()
                # strip whitespaces and quotes
                value = toks[-1].replace('"', '').strip()
                current[key] = value

    return ""

def get_version_ldflags(ctx):
    """
    Compute the version from the git tags, and set the appropriate compiler
    flags
    """
    payload_v = get_payload_version()
    commit = ctx.run("git rev-parse --short HEAD", hide=True).stdout.strip()

    ldflags = "-X {}/pkg/version.Commit={} ".format(REPO_PATH, commit)
    ldflags += "-X {}/pkg/version.AgentVersion={} ".format(REPO_PATH, get_version(ctx, include_git=True))
    ldflags += "-X {}/pkg/serializer.AgentPayloadVersion={} ".format(REPO_PATH, payload_v)
    return ldflags

def get_root():
    """
    Get the root of the Go project
    """
    return check_output(['git', 'rev-parse', '--show-toplevel']).strip()


def get_git_branch_name():
    """
    Return the name of the current git branch
    """
    return check_output(["git", "rev-parse", "--abbrev-ref", "HEAD"]).strip()


def query_version(ctx, git_sha_length=7):
    # The string that's passed in will look something like this: 6.0.0-beta.0-1-g4f19118
    # if the tag is 6.0.0-beta.0, it has been one commit since the tag and that commit hash is g4f19118
    described_version = ctx.run("git describe --tags", hide=True).stdout.strip()
    # For the tag 6.0.0-beta.0, this will match 6.0.0
    version_match = re.findall(r"^(?:dca)?-?v?(\d+\.\d+\.\d+)", described_version)

    if version_match and version_match[0]:
        version = version_match[0]
    else:
        raise Exception("Could not query valid version from tags of local git repository")

    # for the example above, 6.0.0-beta.0-1-g4f19118, this will be 1
    commits_since_version_match = re.findall(r"^.*-(\d+)\-g[0-9a-f]+$", described_version)
    git_sha_match = re.findall(r"g([0-9a-f]+)$", described_version)

    if commits_since_version_match and commits_since_version_match[0]:
        commits_since_version = int(commits_since_version_match[0])
    else:
        commits_since_version = 0

    pre_regex = ""
    # for the output, 6.0.0-beta.0-1-g4f19118, this will match beta.0
    # if there have been no commits since, it will be just 6.0.0-beta.0,
    # and it will match beta.0
    if commits_since_version == 0:
        pre_regex = r"^(?:dca)?-?v?\d+\.\d+\.\d+(?:-|\.)([0-9A-Za-z.-]+)$"
    else:
        pre_regex = r"^(?:dca)?-?v?\d+\.\d+\.\d+(?:-|\.)([0-9A-Za-z.-]+)-\d+-g[0-9a-f]+$"

    pre_match = re.findall(pre_regex, described_version)
    pre = ""
    if pre_match and pre_match[0]:
        pre = pre_match[0]

    # for the output, 6.0.0-beta.0-1-g4f19118, this will match g4f19118
    git_sha = ""
    if git_sha_match and git_sha_match[0]:
        git_sha_long = ctx.run("git rev-parse HEAD", hide=True).stdout.strip()
        git_sha = git_sha_long[:git_sha_length]

    return version, pre, commits_since_version, git_sha


def get_version(ctx, include_git=False, url_safe=False, git_sha_length=7):
    # we only need the git info for the non omnibus builds, omnibus includes all this information by default
    version = ""
    version, pre, commits_since_version, git_sha = query_version(ctx, git_sha_length)
    if pre:
        version = "{0}-{1}".format(version, pre)
    if commits_since_version and include_git:
        if url_safe:
            version = "{0}.git.{1}.{2}".format(version, commits_since_version,git_sha)
        else:
            version = "{0}+git.{1}.{2}".format(version, commits_since_version,git_sha)
    return version

def get_version_numeric_only(ctx):
    version, _, _, _ = query_version(ctx)
    return version

def load_release_versions(ctx, target_version):
    with open("release.json", "r") as f:
        versions = json.load(f)
        if target_version in versions:
            # windows runners don't accepts anything else than strings in the
            # environment when running a subprocess.
            return {str(k):str(v) for k, v in versions[target_version].iteritems()}
    raise Exception("Could not find '{}' version in release.json".format(target_version))
