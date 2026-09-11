# -*- coding: utf-8 -*-

from glustercli.cli.utils import gluster_execute


def enable():
    """
    Enable NFS Ganesha

    :returns: Output of Enable command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["nfs-ganesha", "enable"]
    return gluster_execute(cmd)


def disable():
    """
    Disable NFS Ganesha

    :returns: Output of Disable command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["nfs-ganesha", "disable"]
    return gluster_execute(cmd)
