# stdlib
import logging
import os

# 3rd party
from docker import Client
from docker import tls

class MountException(Exception):
    pass

# Default docker client settings
DEFAULT_TIMEOUT = 5
DEFAULT_VERSION = 'auto'
CHECK_NAME = 'docker_daemon'

log = logging.getLogger(__name__)
_docker_client_settings = {"version": DEFAULT_VERSION}

def is_dockerized():
    return os.environ.get("DOCKER_DD_AGENT") == "yes"

def get_docker_settings():
    global _docker_client_settings
    return _docker_client_settings


def reset_docker_settings():
    global _docker_client_settings
    _docker_client_settings = {"version": DEFAULT_VERSION}


def set_docker_settings(init_config, instance):
    global _docker_client_settings
    _docker_client_settings = {
        "version": init_config.get('api_version', DEFAULT_VERSION),
        "base_url": instance.get("url"),
        "timeout": int(init_config.get('timeout', DEFAULT_TIMEOUT)),
    }

    if init_config.get('tls', False):
        client_cert_path = init_config.get('tls_client_cert')
        client_key_path = init_config.get('tls_client_key')
        cacert = init_config.get('tls_cacert')
        verify = init_config.get('tls_verify')

        client_cert = None
        if client_cert_path is not None and client_key_path is not None:
            client_cert = (client_cert_path, client_key_path)

        verify = verify if verify is not None else cacert
        tls_config = tls.TLSConfig(client_cert=client_cert, verify=verify)
        _docker_client_settings["tls"] = tls_config


def get_client():
    return Client(**_docker_client_settings)


def find_cgroup(hierarchy, docker_root):
        """Find the mount point for a specified cgroup hierarchy.

        Works with old style and new style mounts.
        """
        with open(os.path.join(docker_root, "/proc/mounts"), 'r') as fp:
            mounts = map(lambda x: x.split(), fp.read().splitlines())
        cgroup_mounts = filter(lambda x: x[2] == "cgroup", mounts)
        if len(cgroup_mounts) == 0:
            raise Exception(
                "Can't find mounted cgroups. If you run the Agent inside a container,"
                " please refer to the documentation.")
        # Old cgroup style
        if len(cgroup_mounts) == 1:
            return os.path.join(docker_root, cgroup_mounts[0][1])

        candidate = None
        for _, mountpoint, _, opts, _, _ in cgroup_mounts:
            if hierarchy in opts:
                if mountpoint.startswith("/host/"):
                    return os.path.join(docker_root, mountpoint)
                candidate = mountpoint

        if candidate is not None:
            return os.path.join(docker_root, candidate)
        raise Exception("Can't find mounted %s cgroups." % hierarchy)


def find_cgroup_filename_pattern(mountpoints, container_id):
    # We try with different cgroups so that it works even if only one is properly working
    for mountpoint in mountpoints.itervalues():
        stat_file_path_lxc = os.path.join(mountpoint, "lxc")
        stat_file_path_docker = os.path.join(mountpoint, "docker")
        stat_file_path_coreos = os.path.join(mountpoint, "system.slice")
        stat_file_path_kubernetes = os.path.join(mountpoint, container_id)
        stat_file_path_kubernetes_docker = os.path.join(mountpoint, "system", "docker", container_id)
        stat_file_path_docker_daemon = os.path.join(mountpoint, "docker-daemon", "docker", container_id)

        if os.path.exists(stat_file_path_lxc):
            return os.path.join('%(mountpoint)s/lxc/%(id)s/%(file)s')
        elif os.path.exists(stat_file_path_docker):
            return os.path.join('%(mountpoint)s/docker/%(id)s/%(file)s')
        elif os.path.exists(stat_file_path_coreos):
            return os.path.join('%(mountpoint)s/system.slice/docker-%(id)s.scope/%(file)s')
        elif os.path.exists(stat_file_path_kubernetes):
            return os.path.join('%(mountpoint)s/%(id)s/%(file)s')
        elif os.path.exists(stat_file_path_kubernetes_docker):
            return os.path.join('%(mountpoint)s/system/docker/%(id)s/%(file)s')
        elif os.path.exists(stat_file_path_docker_daemon):
            return os.path.join('%(mountpoint)s/docker-daemon/docker/%(id)s/%(file)s')


    raise MountException("Cannot find Docker cgroup directory. Be sure your system is supported.")


def image_tag_extractor(entity, key):
    if "Image" in entity:
        split = entity["Image"].split(":")
        if len(split) <= key:
            return None
        elif len(split) > 2:
            # if the repo is in the image name and has the form 'docker.clearbit:5000'
            # the split will be like [repo_url, repo_port/image_name, image_tag]. Let's avoid that
            split = [':'.join(split[:-1]), split[-1]]
        return [split[key]]
    if "RepoTags" in entity:
        splits = [el.split(":") for el in entity["RepoTags"]]
        tags = set()
        for split in splits:
            if len(split) > 2:
                split = [':'.join(split[:-1]), split[-1]]
            if len(split) > key:
                tags.add(split[key])
        if len(tags) > 0:
            return list(tags)
    return None


def container_name_extractor(co):
    names = co.get('Names', [])
    if names is not None:
        # we sort the list to make sure that a docker API update introducing
        # new names with a single "/" won't make us report dups.
        names = sorted(names)
        for name in names:
            # the leading "/" is legit, if there's another one it means the name is actually an alias
            if name.count('/') <= 1:
                return [str(name).lstrip('/')]
    return co.get('Id')[:11]


def get_hostname():
    """Return the `Name` param from `docker info` to use as the hostname"""
    from config import get_confd_path, check_yaml, PathNotFound

    confd_path = ''

    try:
        confd_path = get_confd_path()
    except PathNotFound:
        log.error("Couldn't find the check configuration folder, not using the docker hostname.")
        return None

    conf_path = os.path.join(confd_path, '%s.yaml' % CHECK_NAME)
    if not os.path.exists(conf_path):
        default_conf_path = os.path.join(confd_path, '%s.yaml.default' % CHECK_NAME)
        if not os.path.exists(default_conf_path):
            log.error("Couldn't find any configuration file for the docker check."
                      " Not using the docker hostname.")
            return None
        else:
            conf_path = default_conf_path

    check_config = check_yaml(conf_path)
    init_config, instances = check_config.get('init_config', {}), check_config['instances']
    init_config = {} if init_config is None else init_config
    if len(instances) > 0:
        set_docker_settings(init_config, instances[0])
        return get_client().info().get("Name")
    return None
