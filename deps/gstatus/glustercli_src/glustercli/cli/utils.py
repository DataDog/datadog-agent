# -*- coding: utf-8 -*-

import subprocess
import xml.etree.cElementTree as ET
from contextlib import contextmanager
from enum import IntEnum

GLUSTERCMD = "gluster"
GLUSTERD_SOCKET = None
ssh = None
SSH_HOST = None
SSH_PEM_FILE = None
prev_ssh_host = None
prev_ssh_pem_file = None


@contextmanager
def ssh_connection(hostname, pem_file):
    global SSH_HOST, SSH_PEM_FILE
    SSH_HOST = hostname
    SSH_PEM_FILE = pem_file
    yield
    SSH_HOST = None
    SSH_PEM_FILE = None


def execute(cmd):
    global prev_ssh_host, prev_ssh_pem_file

    cmd_args = []
    cmd_args.append(GLUSTERCMD)

    if GLUSTERD_SOCKET:
        cmd_args.append("--glusterd-sock={0}".format(GLUSTERD_SOCKET))

    cmd_args.append("--mode=script")
    cmd_args += cmd

    if SSH_HOST is not None and SSH_PEM_FILE is not None:
        # Reconnect only if first time or previously connected to different
        # host or using different pem key
        if ssh is None or prev_ssh_host != SSH_HOST \
           or prev_ssh_pem_file != SSH_PEM_FILE:
            __connect_ssh()
            prev_ssh_host = SSH_HOST
            prev_ssh_pem_file = SSH_PEM_FILE

        cmd_args = " ".join(cmd_args)
        _, stdout, stderr = ssh.exec_command(cmd_args)
        returncode = stdout.channel.recv_exit_status()
        return (returncode, stdout.read().strip(), stderr.read().strip())

    proc = subprocess.Popen(cmd_args, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE,
                            universal_newlines=True)
    out, err = proc.communicate()
    return (proc.returncode, out, err)


class RebalanceOperationType(IntEnum):
    """
    from rpc/xdr/src/cli1-xdr.x

    Represents the type of rebalance cmds
    that can be issued towards a gluster
    volume.
    (i.e. volume rebalance <gvol> fix-layout start)
    """
    NONE = 0
    START = 1
    STOP = 2
    STATUS = 3
    START_LAYOUT_FIX = 4
    START_FORCE = 5
    START_TIER = 6
    STATUS_TIER = 7
    START_DETACH_TIER = 8
    STOP_DETACH_TIER = 9
    PAUSE_TIER = 10
    RESUME_TIER = 11
    DETACH_STATUS = 12
    STOP_TIER = 13
    DETACH_START = 14
    DETACH_COMMIT = 15
    DETACH_COMMIT_FORCE = 16
    DETACH_STOP = 17
    TYPE_MAX = 18  # unused


class GlusterCmdException(Exception):
    pass


def set_ssh_pem_file(pem_file):
    global USE_SSH, SSH_PEM_FILE
    USE_SSH = True
    SSH_PEM_FILE = pem_file


def set_ssh_host(hostname):
    global SSH_HOST
    SSH_HOST = hostname


def __connect_ssh():
    global ssh

    import paramiko  # noqa # pylint: disable=import-outside-toplevel

    if ssh is None:
        ssh = paramiko.SSHClient()
        try:
            ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
            ssh.connect(SSH_HOST, username="root", key_filename=SSH_PEM_FILE)
        except paramiko.ssh_exception.AuthenticationException as err:
            raise GlusterCmdException("Unable to establish SSH connection "
                                      "to root@{0}:\n{1}".format(
                                          SSH_HOST, err))


def set_gluster_path(path):
    global GLUSTERCMD
    GLUSTERCMD = path


def set_gluster_socket(path):
    global GLUSTERD_SOCKET
    GLUSTERD_SOCKET = path


def check_for_xml_errors(data):
    stdout = data[0]
    stderr = data[1]

    # depending on the gluster sub-command that's run
    # it can have a returncode of 0 (meaning success)
    # however this could mean that the formatting of
    # the xml was successful and not the command that
    # was run. We need to check stdout and/or stderr
    # for the `opRet` xml element and if it's -1, then
    # format the error accordingly and raise
    # GlusterCmdException

    # for reasons unknown, some commands will fail and
    # return to stdout instead of stderr and vice versa
    error = stdout if stdout else stderr or None
    if error is not None:
        try:
            error = ET.fromstring(error)
        except ET.ParseError:
            # means parsing xml data failed
            # so play it safe and ignore
            return

        op_ret = error.find('opRet').text or None
        op_err = error.find('opErrstr').text or None
        if op_ret == '-1':
            if op_err is None:
                # means command failed but no error
                # string so make up one
                op_err = 'FAILED'
            raise GlusterCmdException((int(op_ret), '', op_err))


def execute_or_raise(cmd):
    returncode, out, err = execute(cmd)
    if returncode != 0:
        raise GlusterCmdException((returncode, out, err))

    check_for_xml_errors((out, err))

    return out.strip()


def gluster_system_execute(cmd):
    cmd.insert(0, "system::")
    cmd.insert(1, "execute")
    return execute_or_raise(cmd)


def gluster_execute(cmd):
    return execute_or_raise(cmd)


def gluster_execute_xml(cmd):
    cmd.append("--xml")
    return execute_or_raise(cmd)


def volume_execute(cmd):
    cmd.insert(0, "volume")
    return execute_or_raise(cmd)


def peer_execute(cmd):
    cmd.insert(0, "peer")
    return execute_or_raise(cmd)


def volume_execute_xml(cmd):
    cmd.insert(0, "volume")
    return gluster_execute_xml(cmd)


def peer_execute_xml(cmd):
    cmd.insert(0, "peer")
    return gluster_execute_xml(cmd)


def georep_execute(cmd):
    cmd.insert(0, "volume")
    cmd.insert(1, "geo-replication")
    return execute_or_raise(cmd)


def georep_execute_xml(cmd):
    cmd.insert(0, "volume")
    cmd.insert(1, "geo-replication")
    return gluster_execute_xml(cmd)


def bitrot_execute(cmd):
    cmd.insert(0, "volume")
    cmd.insert(1, "bitrot")
    return execute_or_raise(cmd)


def bitrot_execute_xml(cmd):
    cmd.insert(0, "volume")
    cmd.insert(1, "bitrot")
    return gluster_execute_xml(cmd)


def quota_execute(cmd):
    cmd.insert(0, "volume")
    cmd.insert(1, "quota")
    return execute_or_raise(cmd)


def quota_execute_xml(cmd):
    cmd.insert(0, "volume")
    cmd.insert(1, "quota")
    return gluster_execute_xml(cmd)


def heal_execute(cmd):
    cmd.insert(0, "volume")
    cmd.insert(1, "heal")
    return execute_or_raise(cmd)


def heal_execute_xml(cmd):
    cmd.insert(0, "volume")
    cmd.insert(1, "heal")
    return gluster_execute_xml(cmd)


def snapshot_execute(cmd):
    cmd.insert(0, "snapshot")
    return execute_or_raise(cmd)


def snapshot_execute_xml(cmd):
    cmd.insert(0, "snapshot")
    return gluster_execute_xml(cmd)


def tier_execute(cmd):
    cmd.insert(0, "volume")
    cmd.insert(1, "tier")
    return execute_or_raise(cmd)


def tier_execute_xml(cmd):
    cmd.insert(0, "volume")
    cmd.insert(1, "tier")
    return gluster_execute_xml(cmd)
