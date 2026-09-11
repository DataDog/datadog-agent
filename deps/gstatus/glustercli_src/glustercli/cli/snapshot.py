# -*- coding: utf-8 -*-

from glustercli.cli.utils import snapshot_execute, snapshot_execute_xml
from glustercli.cli.parsers import (parse_snapshot_status,
                                    parse_snapshot_info,
                                    parse_snapshot_list)


def activate(snapname, force=False):
    """
    Activate Snapshot

    :param snapname: Snapshot Name
    :param force: True|False Force Activate the snapshot
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["activate", snapname]

    if force:
        cmd += ["force"]

    return snapshot_execute(cmd)


def clone(clonename, snapname):
    """
    Clone the Snapshot

    :param clonename: Snapshot Clone Name
    :param snapname: Snapshot Name
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["clone", clonename, snapname]

    return snapshot_execute(cmd)


def create(volname, snapname, no_timestamp=False, description="", force=False):
    """
    Create Snapshot

    :param volname: Volume Name
    :param snapname: Snapshot Name
    :param no_timestamp: True|False Do not add Timestamp to name
    :param description: Description for Created Snapshot
    :param force: True|False Force Create the snapshot
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["create", snapname, volname]

    if no_timestamp:
        cmd += ["no-timestamp"]

    if description:
        cmd += ["description", description]

    if force:
        cmd += ["force"]

    return snapshot_execute(cmd)


def deactivate(snapname):
    """
    Deactivate the Snapshot

    :param snapname: Snapshot Name
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["deactivate", snapname]

    return snapshot_execute(cmd)


def delete(snapname=None, volname=None):
    """
    Delete Snapshot

    :param snapname: Snapshot Name
    :param volname: Volume Name
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["delete"]
    if snapname is not None:
        cmd += [snapname]

    if volname is not None and snapname is None:
        cmd += ["volume", volname]

    return snapshot_execute(cmd)


def info(snapname=None, volname=None):
    """
    Snapshot Info

    :param snapname: Snapshot Name
    :param volname: Volume Name
    :returns: Snapshot Info, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["info"]
    if snapname is not None:
        cmd += [snapname]

    if volname is not None and snapname is None:
        cmd += ["volume", volname]

    return parse_snapshot_info(snapshot_execute_xml(cmd))


def snaplist(volname=None):
    """
    List of Snapshots

    :param volname: Volume Name
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["list"]

    if volname is not None:
        cmd += [volname]

    return parse_snapshot_list(snapshot_execute_xml(cmd))


def restore(snapname):
    """
    Restore Snapshot

    :param snapname: Snapshot Name
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["restore", snapname]
    return snapshot_execute(cmd)


def status(snapname=None, volname=None):
    """
    Snapshot Status

    :param snapname: Snapshot Name
    :param volname: Volume Name
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["status"]
    if snapname is not None:
        cmd += [snapname]

    if volname is not None and snapname is None:
        cmd += ["volume", volname]

    return parse_snapshot_status(snapshot_execute_xml(cmd))


def config(volname, snap_max_hard_limit=None,
           snap_max_soft_limit=None, auto_delete=None,
           activate_on_create=None):
    """
    Set Snapshot Config

    :param volname: Volume Name
    :param snap_max_hard_limit: Number of Snapshots hard limit
    :param snap_max_soft_limit: Number of Snapshots soft limit
    :param auto_delete: True|False Auto delete old snapshots
    :param activate_on_create: True|False Activate Snapshot after Create
    :returns: Output of the command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["config", volname]

    if snap_max_hard_limit is not None:
        cmd += ["snap-max-hard-limit", "{0}".format(snap_max_hard_limit)]

    if snap_max_soft_limit is not None:
        cmd += ["snap-max-soft-limit", "{0}".format(snap_max_soft_limit)]

    if auto_delete is not None:
        auto_delete_arg = "enable" if auto_delete else "disable"
        cmd += ["snap-max-hard-limit", auto_delete_arg]

    if activate_on_create is not None:
        activate_arg = "enable" if activate_on_create else "disable"
        cmd += ["snap-max-hard-limit", activate_arg]

    return snapshot_execute(cmd)
