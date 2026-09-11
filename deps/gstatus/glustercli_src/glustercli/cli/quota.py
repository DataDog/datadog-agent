# -*- coding: utf-8 -*-

from glustercli.cli.utils import quota_execute, quota_execute_xml, \
    volume_execute
from glustercli.cli.parsers import parse_quota_list_paths, \
    parse_quota_list_objects


def inode_quota_enable(volname):
    """
    Enable Inode Quota

    :param volname: Volume Name
    :returns: Output of inode-quota Enable command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["inode-quota", volname, "enable"]
    return volume_execute(cmd)


def enable(volname):
    """
    Enable Quota

    :param volname: Volume Name
    :returns: Output of quota Enable command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "enable"]
    return quota_execute(cmd)


def disable(volname):
    """
    Disable Inode Quota

    :param volname: Volume Name
    :returns: Output of quota Disable command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "disable"]
    return quota_execute(cmd)


# noqa # pylint: disable=dangerous-default-value
def list_paths(volname, paths=[]):
    """
    Get Quota List

    :param volname: Volume Name
    :param paths: Optional list of paths
    :returns: Quota list of paths, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "list"] + paths
    return parse_quota_list_paths(quota_execute_xml(cmd))


# noqa # pylint: disable=dangerous-default-value
def list_objects(volname, paths=[]):
    """
    Get Quota Objects List

    :param volname: Volume Name
    :param paths: Optional list of paths
    :returns: Quota list of objects, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "list"] + paths
    return parse_quota_list_objects(quota_execute_xml(cmd))


def remove_path(volname, path):
    """
    Remove Path from Quota list

    :param volname: Volume Name
    :param path: Path to remove from quota
    :returns: Output of Quota remove-path, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "remove-path", path]
    return quota_execute(cmd)


def remove_objects(volname, path):
    """
    Remove Objects for a given path

    :param volname: Volume Name
    :param path: Path to remove from quota
    :returns: Output of Quota remove-objects, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "remove-objects", path]
    return quota_execute(cmd)


def default_soft_limit(volname, percent):
    """
    Set default soft limit

    :param volname: Volume Name
    :param percent: Percent of soft limit
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "default-soft-limit", "{0}".format(percent)]
    return quota_execute(cmd)


def limit_usage(volname, path, size, percent=None):
    """
    Limit quota usage

    :param volname: Volume Name
    :param path: Path to limit quota
    :param size: Limit Size
    :param percent: Percentage
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "limit-usage", path, "{0}".format(size)]
    if percent is not None:
        cmd += ["{0}".format(percent)]
    return quota_execute(cmd)


def limit_objects(volname, path, num, percent=None):
    """
    Limit objects

    :param volname: Volume Name
    :param path: Path to limit quota
    :param num: Limit Number
    :param percent: Percentage
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "limit-objects", path, "{0}".format(num)]
    if percent is not None:
        cmd += ["{0}".format(percent)]
    return quota_execute(cmd)


def alert_time(volname, a_time):
    """
    Set Alert Time

    :param volname: Volume Name
    :param alert_time: Alert Time Value
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "alert-time", "{0}".format(a_time)]
    return quota_execute(cmd)


def soft_timeout(volname, timeout):
    """
    Set Soft Timeout

    :param volname: Volume Name
    :param timeout: Timeout Value
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "soft-timeout", "{0}".format(timeout)]
    return quota_execute(cmd)


def hard_timeout(volname, timeout):
    """
    Set Hard Timeout

    :param volname: Volume Name
    :param timeout: Timeout Value
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname, "hard-timeout", "{0}".format(timeout)]
    return quota_execute(cmd)
