"""
Miscellaneous functions, no tasks here
"""
from __future__ import print_function

import os
import platform
from subprocess import check_output

import invoke


# constants
ORG_PATH = "github.com/DataDog"
REPO_PATH = "{}/datadog-agent".format(ORG_PATH)


def bin_name(name):
    """
    Generate platform dependent names for binaries
    """
    if invoke.platform.WINDOWS:
        return "{}.exe".format(name)
    return "{}.bin".format(name)


def pkg_config_path(use_system_libs):
    """
    Prepend the full path to either the `system` or `embedded` pkg-config
    folders provided by the agent to the existing value of `PKG_CONFIG_PATH`
    environment var. If the env var is not set, do nothing and return an
    empty string.
    """
    retval = ""

    if not os.environ.get("PKG_CONFIG_PATH"):
        return retval

    base = os.path.join(os.path.dirname("."), "pkg-config", platform.system().lower())
    if use_system_libs:
        retval = os.path.abspath(os.path.join(base, "system"))
    else:
        retval = os.path.abspath(os.path.join(base, "embedded"))

    # append the system wide value of PKG_CONFIG_PATH
    retval += ":{}".format(os.environ.get("PKG_CONFIG_PATH"))

    return retval


def get_ldflags(ctx, static=False):
    """
    Build the common value for both ldflags and gcflags.

    We need to invoke external processes here so this function need the
    Context object.
    """
    payload_v = get_payload_version()
    commit = ctx.run("git rev-parse --short HEAD", hide=True).stdout.strip()

    gcflags = ""
    ldflags = "-X {}/pkg/version.commit={} ".format(REPO_PATH, commit)
    ldflags += "-X {}/pkg/serializer.AgentPayloadVersion={} ".format(REPO_PATH, payload_v)
    if static:
        ldflags += "-s -w -linkmode external -extldflags \"-static\" "

    if os.environ.get("DELVE"):
        gcflags = "-N -l"
        if invoke.platform.WINDOWS:
            # On windows, need to build with the extra argument -ldflags="-linkmode internal"
            # if you want to be able to use the delve debugger.
            ldflags += "-linkmode internal "

    return ldflags, gcflags


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


def get_root():
    """
    Get the root of the Go project
    """
    return check_output(['git', 'rev-parse', '--show-toplevel']).strip()
