#!/usr/bin/env python3
"""gstatus — GlusterFS cluster status collector.

Thin entry point that drives the vendored glustercli/glusterlib packages to
collect GlusterFS cluster/volume/brick status and emit it as JSON (or text).

This is a from-source replacement for the upstream PyInstaller-frozen
``gstatus`` binary. The frozen binary bundled a full Python interpreter and
~8 MiB of crypto/SSH dependencies (cryptography, PyNaCl, bcrypt, paramiko,
cffi) that are unnecessary for local status collection — the Datadog glusterfs
integration runs ``gstatus -a -o json -u g`` locally on the gluster node and
never uses SSH remote mode.

Behavior is intentionally identical to the upstream ``gstatus`` for the flags
the Datadog integration relies on (``-a``, ``-o json``, ``-u``).
"""

import os
import sys

from optparse import OptionParser

try:
    from packaging.version import parse as version
except ImportError:  # pragma: no cover - packaging is available in the agent env
    version = None

from glusterlib.cluster import Cluster
from glusterlib.display_status import display_status

GSTATUS_VERSION = "1.0.9"
SUPPORTED_GLUSTER_VERSION = "3.12"


def check_version(cluster):
    """Check the gluster version, exit if less than the supported minimum."""
    if version is None:
        return
    if version(cluster.glusterfs_version) < version(SUPPORTED_GLUSTER_VERSION):
        print(
            "gstatus: GlusterFS %s is not supported, use GlusterFS %s or above."
            % (cluster.glusterfs_version, SUPPORTED_GLUSTER_VERSION),
            file=sys.stderr,
        )
        exit(1)


def parse_options():
    usage = "usage: %prog [options]"
    parser = OptionParser(usage=usage, version="%prog " + GSTATUS_VERSION)

    parser.add_option(
        "-v",
        "--volume",
        dest="volumes",
        action="store_true",
        default=False,
        help="Supply a volume or a list of volumes by repeated invocation of -v.",
    )
    parser.add_option(
        "-a",
        "--all",
        dest="alldata",
        action="store_true",
        default=False,
        help="Print all available details on volumes",
    )
    parser.add_option(
        "-b",
        "--bricks",
        dest="brickinfo",
        action="store_true",
        default=False,
        help="Print the list of bricks",
    )
    parser.add_option(
        "-q",
        "--quota",
        dest="displayquota",
        action="store_true",
        default=False,
        help="Print the quota information",
    )
    parser.add_option(
        "-s",
        "--snapshots",
        dest="displaysnap",
        action="store_true",
        default=False,
        help="Print the snapshot information",
    )
    parser.add_option(
        "-u",
        "--units",
        dest="units",
        choices=["h", "k", "m", "g", "t", "p"],
        help="display storage size in given units",
    )
    parser.add_option(
        "-o",
        "--output-mode",
        dest="output_mode",
        help="Output mode, only json is supported currently. Default is to print to console.",
    )
    return parser.parse_args()


def main():
    options, args = parse_options()
    # Are you root?
    if os.getuid() != 0:
        print(
            "You have to be root or have sudo privileges to run this program.",
            file=sys.stderr,
        )
        exit(1)
    cluster = Cluster(options, args)
    check_version(cluster)
    cluster.gather_data()
    display_status(cluster)


if __name__ == "__main__":
    main()
