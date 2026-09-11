# -*- coding: utf-8 -*-

from glustercli.cli.utils import georep_execute, georep_execute_xml, \
    gluster_system_execute
from glustercli.cli.parsers import parse_georep_config, \
    parse_georep_status
from glustercli.cli import volume


def gsec_create(ssh_key_prefix=True):
    """
    Generate Geo-replication SSH Keys

    :param ssh_key_prefix: True|False Command prefix in generated public keys
    :returns: Output of gsec_create command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = ["gsec_create"]
    if not ssh_key_prefix:
        cmd += ["container"]

    return gluster_system_execute(cmd)


# noqa # pylint: disable=too-many-arguments
def create(volname, slave_host, slave_vol, slave_user="root",
           push_pem=True, no_verify=False, force=False, ssh_port=22):
    """
    Create Geo-replication Session

    :param volname: Master Volume Name
    :param slave_host: Slave Hostname or IP
    :param slave_vol: Slave Volume
    :param slave_user: Slave User, default is "root"
    :param push_pem: True|False Push SSH keys to Slave
    :param no_verify: True|False Skip the Slave Verification
     process before create
    :param force: True|False Force Create Session
    :param ssh_port: SSH Port, Default is 22
    :returns: Output of Create command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname,
           "{0}@{1}::{2}".format(slave_user, slave_host, slave_vol),
           "create"]

    if ssh_port != 22:
        cmd += ["ssh-port", "{0}".format(ssh_port)]

    if push_pem:
        cmd += ["push-pem"]

    if no_verify:
        cmd += ["no-verify"]

    if force:
        cmd += ["force"]

    return georep_execute(cmd)


def start(volname, slave_host, slave_vol, slave_user="root", force=False):
    """
    Start Geo-replication Session

    :param volname: Master Volume Name
    :param slave_host: Slave Hostname or IP
    :param slave_vol: Slave Volume
    :param slave_user: Slave User, default is "root"
    :param force: True|False Force Start the Session
    :returns: Output of Start command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname,
           "{0}@{1}::{2}".format(slave_user, slave_host, slave_vol),
           "start"]

    if force:
        cmd += ["force"]

    return georep_execute(cmd)


def stop(volname, slave_host, slave_vol, slave_user="root", force=False):
    """
    Stop Geo-replication Session

    :param volname: Master Volume Name
    :param slave_host: Slave Hostname or IP
    :param slave_vol: Slave Volume
    :param slave_user: Slave User, default is "root"
    :param force: True|False Force Stop the Session
    :returns: Output of Stop command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname,
           "{0}@{1}::{2}".format(slave_user, slave_host, slave_vol),
           "stop"]

    if force:
        cmd += ["force"]

    return georep_execute(cmd)


def restart(volname, slave_host, slave_vol, slave_user="root", force=False):
    """
    Restart Geo-replication Session

    :param volname: Master Volume Name
    :param slave_host: Slave Hostname or IP
    :param slave_vol: Slave Volume
    :param slave_user: Slave User, default is "root"
    :param force: True|False Force Start the Session
    :returns: Output of Start command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    stop(volname, slave_host, slave_vol, slave_user, force=True)
    return start(volname, slave_host, slave_vol, slave_user, force)


def delete(volname, slave_host, slave_vol, slave_user="root",
           reset_sync_time=None):
    """
    Delete Geo-replication Session

    :param volname: Master Volume Name
    :param slave_host: Slave Hostname or IP
    :param slave_vol: Slave Volume
    :param slave_user: Slave User, default is "root"
    :param reset_sync_time: True|False Reset Sync time on delete
    :returns: Output of Start command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname,
           "{0}@{1}::{2}".format(slave_user, slave_host, slave_vol),
           "delete"]

    if reset_sync_time is not None:
        cmd += ["reset-sync-time"]

    return georep_execute(cmd)


def pause(volname, slave_host, slave_vol, slave_user="root", force=False):
    """
    Pause Geo-replication Session

    :param volname: Master Volume Name
    :param slave_host: Slave Hostname or IP
    :param slave_vol: Slave Volume
    :param slave_user: Slave User, default is "root"
    :param force: True|False Force Pause Session
    :returns: Output of Pause command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname,
           "{0}@{1}::{2}".format(slave_user, slave_host, slave_vol),
           "pause"]

    if force:
        cmd += ["force"]

    return georep_execute(cmd)


def resume(volname, slave_host, slave_vol, slave_user="root", force=False):
    """
    Resume Geo-replication Session

    :param volname: Master Volume Name
    :param slave_host: Slave Hostname or IP
    :param slave_vol: Slave Volume
    :param slave_user: Slave User, default is "root"
    :param force: True|False Force Resume Session
    :returns: Output of Resume command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname,
           "{0}@{1}::{2}".format(slave_user, slave_host, slave_vol),
           "resume"]

    if force:
        cmd += ["force"]

    return georep_execute(cmd)


def config_set(volname, slave_host, slave_vol, key, value,
               slave_user="root"):
    """
    Set Config of a Geo-replication Session

    :param volname: Master Volume Name
    :param slave_host: Slave Hostname or IP
    :param slave_vol: Slave Volume
    :param slave_user: Slave User, default is "root"
    :param key: Config Key
    :param value: Config Value
    :returns: Output of Config set command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname,
           "{0}@{1}::{2}".format(slave_user, slave_host, slave_vol),
           "config", key, value]
    return georep_execute(cmd)


def config_reset(volname, slave_host, slave_vol, key, slave_user="root"):
    """
    Reset configuration of Geo-replication Session

    :param volname: Master Volume Name
    :param slave_host: Slave Hostname or IP
    :param slave_vol: Slave Volume
    :param slave_user: Slave User, default is "root"
    :param key: Config Key
    :returns: Output of Config reset command, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname,
           "{0}@{1}::{2}".format(slave_user, slave_host, slave_vol),
           "config", "!{0}".format(key)]
    return georep_execute(cmd)


def config_get(volname, slave_host, slave_vol, key=None,
               slave_user="root"):
    """
    Get Configuration of Geo-replication Session

    :param volname: Master Volume Name
    :param slave_host: Slave Hostname or IP
    :param slave_vol: Slave Volume
    :param slave_user: Slave User, default is "root"
    :param key: Config Key
    :returns: Geo-rep session Config Values, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = [volname,
           "{0}@{1}::{2}".format(slave_user, slave_host, slave_vol),
           "config"]

    if key is not None:
        cmd += [key]

    return parse_georep_config(georep_execute_xml(cmd))


def status(volname=None, slave_host=None, slave_vol=None,
           slave_user="root"):
    """
    Status of Geo-replication Session

    :param volname: Master Volume Name
    :param slave_host: Slave Hostname or IP
    :param slave_vol: Slave Volume
    :param slave_user: Slave User, default is "root"
    :returns: Geo-replication Status, raises
     GlusterCmdException((rc, out, err)) on error
    """
    cmd = []

    if volname is not None:
        cmd += [volname]

    if volname is not None and slave_host is not None and \
       slave_vol is not None:
        cmd += ["{0}@{1}::{2}".format(slave_user, slave_host, slave_vol)]

    cmd += ["status"]

    return parse_georep_status(georep_execute_xml(cmd), volume.info())
