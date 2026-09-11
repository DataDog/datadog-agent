# -*- coding: utf-8 -*-

from glustercli.cli.utils import volume_execute, volume_execute_xml, \
    GlusterCmdException
from glustercli.cli.parsers import (parse_volume_info,
                                    parse_volume_status,
                                    parse_volume_options,
                                    parse_volume_list,
                                    parse_volume_profile_info)

# Following import are not used in this file, but imported to make
# it available via volume.(noqa to ignore in pep8 check)
from glustercli.cli import bitrot     # noqa # pylint: disable=unused-import
from glustercli.cli import bricks     # noqa # pylint: disable=unused-import
from glustercli.cli import heal       # noqa # pylint: disable=unused-import
from glustercli.cli import quota      # noqa # pylint: disable=unused-import
from glustercli.cli import rebalance  # noqa # pylint: disable=unused-import


LOCK_KINDS = ["blocked", "granted", "all"]
INFO_OPS = ["peek", "incremental", "cumulative", "clear"]


def start(volname, force=False):
    """
    Start Gluster Volume

    :param volname: Volume Name
    :param force: (True|False) Start Volume with Force option
    :returns: Output of Start command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["start", volname]
    if force:
        cmd += ["force"]

    return volume_execute(cmd)


def stop(volname, force=False):
    """
    Stop Gluster Volume

    :param volname: Volume Name
    :param force: (True|False) Stop Volume with Force option
    :returns: Output of Stop command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["stop", volname]
    if force:
        cmd += ["force"]

    return volume_execute(cmd)


def restart(volname, force=False):
    """
    Restart Gluster Volume, Wrapper around two calls stop and start

    :param volname: Volume Name
    :param force: (True|False) Restart Volume with Force option
    :returns: Output of Start command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["stop", volname]
    if force:
        cmd += ["force"]

    volume_execute(cmd)

    cmd = ["start", volname]
    if force:
        cmd += ["force"]

    return volume_execute(cmd)


def delete(volname):
    """
    Delete Gluster Volume

    :param volname: Volume Name
    :returns: Output of Delete command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["delete", volname]
    return volume_execute(cmd)


# noqa # pylint: disable=too-many-arguments
def create(volname, volbricks, replica=0, stripe=0, arbiter=0, disperse=0,
           disperse_data=0, redundancy=0, transport="tcp", force=False):
    """
    Create Gluster Volume

    :param volname: Volume Name
    :param volbricks: List of Brick paths(HOSTNAME:PATH)
    :param replica: Number of Replica bricks
    :param stripe: Number of Stripe bricks
    :param arbiter: Number of Arbiter bricks
    :param disperse: Number of disperse bricks
    :param disperse_data: Number of disperse data bricks
    :param redundancy: Number of Redundancy bricks
    :param transport: Transport mode(tcp|rdma|tcp,rdma)
    :param force: (True|False) Create Volume with Force option
    :returns: Output of Create command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["create", volname]
    if replica != 0:
        cmd += ["replica", "{0}".format(replica)]

    if stripe != 0:
        cmd += ["stripe", "{0}".format(stripe)]

    if arbiter != 0:
        cmd += ["arbiter", "{0}".format(arbiter)]

    if disperse != 0:
        cmd += ["disperse", "{0}".format(disperse)]

    if disperse_data != 0:
        cmd += ["disperse-data", "{0}".format(disperse_data)]

    if redundancy != 0:
        cmd += ["redundancy", "{0}".format(redundancy)]

    if transport != "tcp":
        cmd += ["transport", transport]

    cmd += volbricks

    if force:
        cmd += ["force"]

    return volume_execute(cmd)


def info(volname=None, group_subvols=False):
    """
    Get Gluster Volume Info

    :param volname: Volume Name
    :param group_subvols: Show Subvolume Information in Groups
    :returns: Returns Volume Info, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["info"]
    if volname is not None:
        cmd += [volname]

    return parse_volume_info(volume_execute_xml(cmd),
                             group_subvols=group_subvols)


def status_detail(volname=None, group_subvols=False):
    """
    Get Gluster Volume Status

    :param volname: Volume Name or List of volumes
    :param group_subvols: Show Subvolume Information in Groups
    :returns: Returns Volume Status, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["status"]
    if volname is not None:
        if type(volname) == list:
            volumes = []
            volinfo = {}

            for vi in info():
                volinfo[vi['name']] = vi

            for v in volname:
                cmd += [v, "detail"]
                volumes.append(parse_volume_status(volume_execute_xml(cmd),
                                                   [volinfo[v]],
                                                   group_subvols=\
                                                   group_subvols).pop())
                cmd = ["status"]
            return(volumes)
        else:
            cmd += [volname, "detail"]
    else:
        cmd += ["all", "detail"]

    return parse_volume_status(volume_execute_xml(cmd),
                               info(volname),
                               group_subvols=group_subvols)


def optset(volname, opts):
    """
    Set Volume Options

    :param volname: Volume Name
    :param opts: Dict with config key as dict key and config value as value
    :returns: Output of Volume Set command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["set", volname]
    for key, value in opts.items():
        cmd += [key, value]

    return volume_execute(cmd)


def optget(volname, opt="all"):
    """
    Get Volume Options

    :param volname: Volume Name
    :param opt: Option Name
    :returns: List of Volume Options, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["get", volname, opt]
    return parse_volume_options(volume_execute_xml(cmd))


def optreset(volname, opt=None, force=False):
    """
    Reset Volume Options

    :param volname: Volume Name
    :param opt: Option name to reset, else reset all
    :param force: Force reset options
    :returns: Output of Volume Reset command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["reset", volname]

    if opt is not None:
        cmd += [opt]

    if force:
        cmd += ["force"]

    return volume_execute(cmd)


def vollist():
    """
    Volumes List

    :returns: List of Volumes, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["list"]
    return parse_volume_list(volume_execute_xml(cmd))


def log_rotate(volname, brick):
    """
    Brick log rotate

    :param volname: Volume Name
    :param brick: Brick Path
    :returns: Output of Log rotate command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["log", volname, "rotate", brick]
    return volume_execute(cmd)


def sync(hostname, volname=None):
    """
    Sync the volume information from a peer

    :param hostname: Hostname to sync from
    :param volname: Volume Name
    :returns: Output of Sync command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["sync", hostname]
    if volname is not None:
        cmd += [volname]
    return volume_execute(cmd)


# noqa # pylint: disable=too-many-arguments
def clear_locks(volname, path, kind, inode_range=None,
                entry_basename=None, posix_range=None):
    """
    Clear locks held on path

    :param volname: Volume Name
    :param path: Locked Path
    :param kind: Lock Kind(blocked|granted|all)
    :param inode_range: Inode Range
    :param entry_basename: Entry Basename
    :param posix_range: Posix Range
    :returns: Output of Clear locks command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    if kind.lower() not in LOCK_KINDS:
        raise GlusterCmdException((-1, "", "Invalid Lock Kind"))
    cmd = ["clear-locks", volname, "kind", kind.lower()]

    if inode_range is not None:
        cmd += ["inode", inode_range]

    if entry_basename is not None:
        cmd += ["entry", entry_basename]

    if posix_range is not None:
        cmd += ["posix", posix_range]

    return volume_execute(cmd)


def barrier_enable(volname):
    """
    Enable Barrier

    :param volname: Volume Name
    :returns: Output of Barrier command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["barrier", volname, "enable"]
    return volume_execute(cmd)


def barrier_disable(volname):
    """
    Disable Barrier

    :param volname: Volume Name
    :returns: Output of Barrier command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["barrier", volname, "disable"]
    return volume_execute(cmd)


def profile_start(volname):
    """
    Start Profile

    :param volname: Volume Name
    :return: Output of Profile command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["profile", volname, "start"]
    return volume_execute(cmd)


def profile_stop(volname):
    """
    Stop Profile

    :param volname: Volume Name
    :return: Output of Profile command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["profile", volname, "stop"]
    return volume_execute(cmd)


def profile_info(volname, opt, peek=False):
    """
    Get Profile info

    :param volname: Volume Name
    :param opt: Operation type of info,
     like peek, incremental, cumulative, clear
    :param peek: Use peek or not, default is False
    :return: Return profile info, raises
     GlusterCmdException((rc, out, err)) on error
    """

    if opt.lower() not in INFO_OPS:
        raise GlusterCmdException((
            -1,
            "",
            "Invalid Info Operation Type, use peek, "
            "incremental, cumulative, clear"
        ))
    cmd = ["profile", volname, "info", opt.lower()]

    if opt.lower() == INFO_OPS[1] and peek:
        cmd += ["peek"]

    return parse_volume_profile_info(volume_execute_xml(cmd), opt)

# TODO: Pending Wrappers
# volume statedump <VOLNAME> [nfs|quotad] [all|mem|iobuf|
#     callpool|priv|fd|inode|history]... - perform statedump on bricks
# volume status [all | <VOLNAME> [nfs|shd|<BRICK>|quotad]]
#     [detail|clients|mem|inode|fd|callpool|tasks] - display status of
#     all or specified volume(s)/brick
# volume top <VOLNAME> {open|read|write|opendir|readdir|clear}
#     [nfs|brick <brick>] [list-cnt <value>] |
# volume top <VOLNAME> {read-perf|write-perf} [bs <size> count
#     <count>] [brick <brick>] [list-cnt <value>] - volume top operations
