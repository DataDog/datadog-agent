# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# stdlib
import logging
import re
import socket

# project
from utils.cloud_metadata import EC2, GCE
from utils.dockerutil import DockerUtil
from utils.kubernetes import KubeUtil
from utils.platform import Platform
from utils.subprocess_output import get_subprocess_output

VALID_HOSTNAME_RFC_1123_PATTERN = re.compile(r"^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$")
MAX_HOSTNAME_LEN = 255

log = logging.getLogger(__name__)

def is_valid_hostname(hostname):
    if hostname.lower() in set([
        'localhost',
        'localhost.localdomain',
        'localhost6.localdomain6',
        'ip6-localhost',
    ]):
        log.warning("Hostname: %s is local" % hostname)
        return False
    if len(hostname) > MAX_HOSTNAME_LEN:
        log.warning("Hostname: %s is too long (max length is  %s characters)" % (hostname, MAX_HOSTNAME_LEN))
        return False
    if VALID_HOSTNAME_RFC_1123_PATTERN.match(hostname) is None:
        log.warning("Hostname: %s is not complying with RFC 1123" % hostname)
        return False
    return True

def _get_hostname_unix():
    try:
        # try fqdn
        out, _, rtcode = get_subprocess_output(['/bin/hostname', '-f'], log)
        if rtcode == 0:
            return out.strip()
    except Exception:
        return None

def get_hostname(config=None):
    """
    Get the canonical host name this agent should identify as. This is
    the authoritative source of the host name for the agent.

    Tries, in order:

      * agent config (datadog.conf, "hostname:")
      * 'hostname -f' (on unix)
      * socket.gethostname()
    """
    hostname = None

    # first, try the config
    if config is None:
        from config import get_config
        config = get_config(parse_args=True)
    config_hostname = config.get('hostname')
    if config_hostname and is_valid_hostname(config_hostname):
        return config_hostname

    # Try to get GCE instance name
    gce_hostname = GCE.get_hostname(config)
    if gce_hostname is not None:
        if is_valid_hostname(gce_hostname):
            return gce_hostname

    # Try to get the docker hostname
    if Platform.is_containerized():

        # First we try from the Docker API
        docker_util = DockerUtil()
        docker_hostname = docker_util.get_hostname(use_default_gw=False)
        if docker_hostname is not None and is_valid_hostname(docker_hostname):
            hostname = docker_hostname

        elif Platform.is_k8s(): # Let's try from the kubelet
            kube_util = KubeUtil()
            _, kube_hostname = kube_util.get_node_info()
            if kube_hostname is not None and is_valid_hostname(kube_hostname):
                hostname = kube_hostname

    # then move on to os-specific detection
    if hostname is None:
        if Platform.is_unix() or Platform.is_solaris():
            unix_hostname = _get_hostname_unix()
            if unix_hostname and is_valid_hostname(unix_hostname):
                hostname = unix_hostname

    # if we don't have a hostname, or we have an ec2 default hostname,
    # see if there's an instance-id available
    if not Platform.is_windows() and (hostname is None or Platform.is_ecs_instance() or EC2.is_default(hostname)):
        instanceid = EC2.get_instance_id(config)
        if instanceid:
            hostname = instanceid

    # fall back on socket.gethostname(), socket.getfqdn() is too unreliable
    if hostname is None:
        try:
            socket_hostname = socket.gethostname()
        except socket.error:
            socket_hostname = None
        if socket_hostname and is_valid_hostname(socket_hostname):
            hostname = socket_hostname

    if hostname is None:
        log.critical('Unable to reliably determine host name. You can define one in datadog.conf or in your hosts file')
        raise Exception('Unable to reliably determine host name. You can define one in datadog.conf or in your hosts file')

    return hostname
