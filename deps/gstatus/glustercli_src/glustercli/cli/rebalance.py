# -*- coding: utf-8 -*-

from glustercli.cli.utils import volume_execute, volume_execute_xml
from glustercli.cli.parsers import parse_rebalance_status


def fix_layout_start(volname):
    """
    Fix Layout Rebalance Start

    :param volname: Volume Name
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["rebalance", volname, "fix-layout", "start"]
    return volume_execute(cmd)


def start(volname, force=False):
    """
    Rebalance Start

    :param volname: Volume Name
    :param force: True|False Force start the rebalance
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["rebalance", volname, "start"]
    if force:
        cmd += ["force"]
    return volume_execute(cmd)


def stop(volname):
    """
    Rebalance Stop

    :param volname: Volume Name
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["rebalance", volname, "stop"]
    return volume_execute(cmd)


def status(volname):
    """
    Rebalance Status

    :param volname: Volume Name
    :returns: Rebalance Status, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["rebalance", volname, "status"]
    return parse_rebalance_status(volume_execute_xml(cmd))
