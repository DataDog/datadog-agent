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

def get_gopath(ctx):
    gopath = os.environ.get("GOPATH")
    if not gopath:
        gopath = ctx.run("go env GOPATH", hide=True).stdout.strip()

    return gopath

def get_multi_python_location(embedded_path=None, rtloader_root=None):
    if rtloader_root is None:
        rtloader_lib = "{}/lib".format(rtloader_root or embedded_path)
        rtloader_headers = "{}/include".format(rtloader_root or embedded_path)
        rtloader_common_headers = "{}/common".format(rtloader_root or embedded_path)
    # if rtloader_root is specified we're working in dev mode from the rtloader folder
    else:
        rtloader_lib = "{}/rtloader".format(rtloader_root)
        rtloader_headers = "{}/include".format(rtloader_root)
        rtloader_common_headers = "{}/common".format(rtloader_root)

    return rtloader_lib, rtloader_headers, rtloader_common_headers

def get_build_flags(ctx, static=False, prefix=None, embedded_path=None,
                    rtloader_root=None, python_home_2=None, python_home_3=None):
    """
    Build the common value for both ldflags and gcflags, and return an env accordingly.

    We need to invoke external processes here so this function need the
    Context object.
    """
    gcflags = ""
    ldflags = get_version_ldflags(ctx, prefix)
    env = {}

    if sys.platform == 'win32':
        env["CGO_LDFLAGS_ALLOW"] = "-Wl,--allow-multiple-definition"

    if embedded_path is None:
        # fall back to local dev path
        embedded_path = "{}/src/github.com/DataDog/datadog-agent/dev".format(get_gopath(ctx))

    rtloader_lib, rtloader_headers, rtloader_common_headers = \
        get_multi_python_location(embedded_path, rtloader_root)

    # setting python homes in the code
    if python_home_2:
        ldflags += "-X {}/pkg/collector/python.pythonHome2={} ".format(REPO_PATH, python_home_2)
    if python_home_3:
        ldflags += "-X {}/pkg/collector/python.pythonHome3={} ".format(REPO_PATH, python_home_3)

    # adding rtloader libs and headers to the env
    env['DYLD_LIBRARY_PATH'] = os.environ.get('DYLD_LIBRARY_PATH', '') + ":{}".format(rtloader_lib) # OSX
    env['LD_LIBRARY_PATH'] = os.environ.get('LD_LIBRARY_PATH', '') + ":{}".format(rtloader_lib) # linux
    env['CGO_LDFLAGS'] = os.environ.get('CGO_LDFLAGS', '') + " -L{}".format(rtloader_lib)
    env['CGO_CFLAGS'] = os.environ.get('CGO_CFLAGS', '') + " -w -I{} -I{}".format(rtloader_headers,
                                                                                  rtloader_common_headers)

    # if `static` was passed ignore setting rpath, even if `embedded_path` was passed as well
    if static:
        ldflags += "-s -w -linkmode=external '-extldflags=-static' "
    else:
        ldflags += "-r {}/lib ".format(embedded_path)

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

def get_version_ldflags(ctx, prefix=None):
    """
    Compute the version from the git tags, and set the appropriate compiler
    flags
    """
    payload_v = get_payload_version()
    commit = get_git_commit()

    ldflags = "-X {}/pkg/version.Commit={} ".format(REPO_PATH, commit)
    ldflags += "-X {}/pkg/version.AgentVersion={} ".format(REPO_PATH, get_version(ctx, include_git=True, prefix=prefix))
    ldflags += "-X {}/pkg/serializer.AgentPayloadVersion={} ".format(REPO_PATH, payload_v)
    return ldflags

def get_git_commit():
    """
    Get the current commit
    """
    return check_output(['git', 'rev-parse', '--short', 'HEAD']).decode('utf-8').strip()

def get_go_version():
    """
    Get the version of Go used
    """
    return check_output(['go', 'version']).decode('utf-8').strip()

def get_root():
    """
    Get the root of the Go project
    """
    return check_output(['git', 'rev-parse', '--show-toplevel']).decode('utf-8').strip()


def get_git_branch_name():
    """
    Return the name of the current git branch
    """
    return check_output(["git", "rev-parse", "--abbrev-ref", "HEAD"]).decode('utf-8').strip()


def query_version(ctx, git_sha_length=7, prefix=None):
    # The string that's passed in will look something like this: 6.0.0-beta.0-1-g4f19118
    # if the tag is 6.0.0-beta.0, it has been one commit since the tag and that commit hash is g4f19118
    cmd = "git describe --tags --candidates=50"
    if prefix and type(prefix) == str:
        cmd += " --match \"{}-*\"".format(prefix)
    else:
        cmd += " --match \"[0-9]*\""
    if git_sha_length and type(git_sha_length) == int:
        cmd += " --abbrev={}".format(git_sha_length)
    described_version = ctx.run(cmd, hide=True).stdout.strip()

    # for the example above, 6.0.0-beta.0-1-g4f19118, this will be 1
    commit_number_match = re.match(r"^.*-(?P<commit_number>\d+)-g[0-9a-f]+$", described_version)
    commit_number = 0
    if commit_number_match:
        commit_number = int(commit_number_match.group('commit_number'))

    version_re = r"v?(?P<version>\d+\.\d+\.\d+)(?:(?:-|\.)(?P<pre>[0-9A-Za-z.-]+))?"
    if prefix and type(prefix) == str:
        version_re = r"^(?:{}-)?".format(prefix) + version_re
    else:
        version_re = r"^" + version_re
    if commit_number == 0:
        version_re += r"(?P<git_sha>)$"
    else:
        version_re += r"-\d+-g(?P<git_sha>[0-9a-f]+)$"

    version_match = re.match(
            version_re,
            described_version)

    if not version_match:
        raise Exception("Could not query valid version from tags of local git repository")

    # version: for the tag 6.0.0-beta.0, this will match 6.0.0
    # pre: for the output, 6.0.0-beta.0-1-g4f19118, this will match beta.0
    # if there have been no commits since, it will be just 6.0.0-beta.0,
    # and it will match beta.0
    # git_sha: for the output, 6.0.0-beta.0-1-g4f19118, this will match g4f19118
    version, pre, git_sha = version_match.group('version', 'pre', 'git_sha')
    return version, pre, commit_number, git_sha


def get_version(ctx, include_git=False, url_safe=False, git_sha_length=7, prefix=None):
    # we only need the git info for the non omnibus builds, omnibus includes all this information by default
    version = ""
    version, pre, commits_since_version, git_sha = query_version(ctx, git_sha_length, prefix)
    if pre:
        version = "{0}-{1}".format(version, pre)
    if commits_since_version and include_git:
        if url_safe:
            version = "{0}.git.{1}.{2}".format(version, commits_since_version,git_sha)
        else:
            version = "{0}+git.{1}.{2}".format(version, commits_since_version,git_sha)

    # version could be unicode as it comes from `query_version`
    return str(version)

def get_version_numeric_only(ctx):
    version, _, _, _ = query_version(ctx)
    return version

def load_release_versions(ctx, target_version):
    with open("release.json", "r") as f:
        versions = json.load(f)
        if target_version in versions:
            # windows runners don't accepts anything else than strings in the
            # environment when running a subprocess.
            return {str(k):str(v) for k, v in versions[target_version].items()}
    raise Exception("Could not find '{}' version in release.json".format(target_version))
