# -*- coding: utf-8 -*-

from glustercli.cli.utils import heal_execute, heal_execute_xml, \
    GlusterCmdException
from glustercli.cli.parsers import parse_heal_statistics, parse_heal_info


HEAL_INFO_TYPES = ["healed", "heal-failed", "split-brain"]


def enable(volname):
    """
    Enable Volume Heal

    :param volname: Volume Name
    :returns: Output of Enable command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "enable"]
    return heal_execute(cmd)


def disable(volname):
    """
    Disable Volume Heal

    :param volname: Volume Name
    :returns: Output of Disable command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "disable"]
    return heal_execute(cmd)


def full(volname):
    """
    Full Volume Heal

    :param volname: Volume Name
    :returns: Output of Full Heal command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "full"]
    return heal_execute(cmd)


def statistics(volname):
    """
    Get Statistics of Heal

    :param volname: Volume Name
    :returns: Output of Statistics command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "statistics"]
    return parse_heal_statistics(heal_execute_xml(cmd))


def info(volname, info_type=None):
    """
    Get Volume Heal Info

    :param volname: Volume Name
    :returns: Output of Heal Info command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "info"]

    if info_type is not None:
        if info_type.lower() not in HEAL_INFO_TYPES:
            raise GlusterCmdException((-1, "", "Invalid Heal Info Types"))

        cmd += [info_type.lower()]

    return parse_heal_info(heal_execute_xml(cmd))


def split_brain(volname, bigger_file=None,
                latest_mtime=None, source_brick=None, path=None):
    """
    Split Brain Resolution

    :param volname: Volume Name
    :param bigger_file: File Path of Bigger file
    :param latest_mtime: File Path of Latest mtime
    :param source_brick: Source Brick for Good Copy
    :param path: Resolution of this path/file
    :returns: Output of Split-brain command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "split-brain"]
    if bigger_file is not None:
        cmd += ["bigger-file", bigger_file]

    if latest_mtime is not None:
        cmd += ["latest-mtime", latest_mtime]

    if source_brick is not None:
        cmd += ["source-brick", source_brick]

    if path is not None:
        cmd += [path]

    return heal_execute(cmd)
