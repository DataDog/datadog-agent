# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# stdlib
import logging
import os
import re
import socket
import struct
import time

# 3rd party
from docker import Client, tls
from docker.errors import NotFound

# project
from utils.singleton import Singleton

SWARM_SVC_LABEL = 'com.docker.swarm.service.name'
RANCHER_CONTAINER_NAME = 'io.rancher.container.name'
RANCHER_CONTAINER_IP = 'io.rancher.container.ip'
RANCHER_STACK_NAME = 'io.rancher.stack.name'
RANCHER_SVC_NAME = 'io.rancher.stack_service.name'
DATADOG_ID = 'com.datadoghq.sd.check.id'


class BogusPIDException(Exception):
    pass


class MountException(Exception):
    pass


class CGroupException(Exception):
    pass


# Default docker client settings
DEFAULT_TIMEOUT = 5
DEFAULT_VERSION = 'auto'
CHECK_NAME = 'docker_daemon'
CONFIG_RELOAD_STATUS = ['start', 'die', 'stop', 'kill']  # used to trigger service discovery
DEFAULT_INIT_RETRIES = 0
DEFAULT_RETRY_INTERVAL = 20  # seconds

# only used if no exclude rule was defined
DEFAULT_CONTAINER_EXCLUDE = ["docker_image:gcr.io/google_containers/pause.*", "image_name:openshift/origin-pod"]

log = logging.getLogger(__name__)


class DockerUtil:
    __metaclass__ = Singleton

    DEFAULT_SETTINGS = {"version": DEFAULT_VERSION}
    DEFAULT_PROCFS_GW_PATH = "proc/net/route"

    def __init__(self, **kwargs):
        self._docker_root = None
        self.events = []
        self.hostname = None
        self._default_gateway = None

        # Keeps record of conffile lookup error, to disable log spam when
        # the docker integration is not configured (dockerless host)
        self._no_conf_file = False

        if 'init_config' in kwargs and 'instance' in kwargs:
            init_config = kwargs.get('init_config')
            instance = kwargs.get('instance')
        else:
            init_config, instance = self.get_check_config()
        self.set_docker_settings(init_config, instance)
        self.last_init_retry = None
        self.init_retry_interval = init_config.get('init_retry_interval', DEFAULT_RETRY_INTERVAL)
        self.left_init_retries = init_config.get('init_retries', DEFAULT_INIT_RETRIES) + 1
        self._client = None

        # At first run we'll just collect the events from the latest 60 secs
        self._latest_event_collection_ts = int(time.time()) - 60

        # Memory cache for sha256 to image name mapping
        self._image_sha_to_name_mapping = {}

        # orchestrator detection
        self._is_ecs = False
        self._is_rancher = False
        self._is_k8s = False

        # if we can't reach docker server there's no point in trying
        if self.client:
            self._init_orchestrators()

        # Build include/exclude patterns for containers
        self._include, self._exclude = instance.get('include', []), instance.get('exclude', [])
        if not self._exclude:
            # In Kubernetes, pause containers are not interesting to monitor.
            # This part could be reused for other platforms where containers can be safely ignored.
            if self.is_k8s():
                self.filtering_enabled = True
                self._exclude = DEFAULT_CONTAINER_EXCLUDE
            else:
                if self._include:
                    log.warning("You must specify an exclude section to enable filtering")
                self.filtering_enabled = False
        else:
            self.filtering_enabled = True

        if self.filtering_enabled:
            self.build_filters()

    @property
    def client(self):
        if self._client:
            return self._client

        # tried too many times, failing permanently
        if self.left_init_retries == 0:
            raise Exception("Docker client initialization failed permanently. "
                "Docker-related features will fail.")

        now = time.time()

        # last retry was less than retry_interval ago
        if self.last_init_retry and now <= self.last_init_retry + self.init_retry_interval:
            return None
        # else it's the first try, or last retry was long enough ago
        self.last_init_retry = now
        self.left_init_retries -= 1
        try:
            self._client = Client(**self.settings)
            self._client.ping()
            return self._client
        except Exception as ex:
            log.error("Failed to initialize the docker client. Docker-related features "
                "will fail. Will retry %s time(s). Error: %s" % (self.left_init_retries, str(ex)))
            self._client = None
            return

    def _init_orchestrators(self):
        # Try to detect if we are on Swarm
        self.fetch_swarm_state()
        try:
            containers = self.client.containers()
            for co in containers:
                if '/ecs-agent' in co.get('Names', ''):
                    self._is_ecs = True
                    break
                elif '/rancher-agent' in co.get('Names', ''):
                    self._is_rancher = True
                    break
        except Exception as e:
            if self._no_conf_file:
                log.debug("Error while detecting orchestrator: %s" % e)
            else:
                log.warning("Error while detecting orchestrator: %s" % e)
            pass

        try:
            from utils.kubernetes import detect_is_k8s
            self._is_k8s = detect_is_k8s()
        except Exception:
            self._is_k8s = False

    def get_check_config(self):
        """Read the config from docker_daemon.yaml"""
        from utils.check_yaml import check_yaml
        from utils.checkfiles import get_conf_path
        init_config, instances = {}, []
        try:
            conf_path = get_conf_path(CHECK_NAME)
        except IOError:
            log.debug("Couldn't find docker settings, trying with defaults.")
            self._no_conf_file = True
            return init_config, {}

        if conf_path is not None and os.path.exists(conf_path):
            try:
                check_config = check_yaml(conf_path)
                init_config, instances = check_config.get('init_config', {}), check_config['instances']
                init_config = {} if init_config is None else init_config
            except Exception:
                log.exception('Docker check configuration file is invalid. The docker check and '
                              'other Docker related components will not work.')
                init_config, instances = {}, []

        if len(instances) > 0:
            instance = instances[0]
        else:
            instance = {}
            log.error('No instance was found in the docker check configuration.'
                      ' Docker related collection will not work.')
        return init_config, instance

    def is_ecs(self):
        return self._is_ecs

    def is_rancher(self):
        return self._is_rancher

    def is_k8s(self):
        return self._is_k8s

    def is_swarm(self):
        if self.swarm_node_state == 'pending':
            self.fetch_swarm_state()
        if self.swarm_node_state == 'active':
            return True
        else:
            return False

    def fetch_swarm_state(self):
        self.swarm_node_state = None
        try:
            info = self.client.info()
            self.swarm_node_state = info.get('Swarm', {}).get('LocalNodeState')
        except Exception:
            pass

    def get_events(self):
        self.events = []
        changed_container_ids = set()
        if not self.client:
            self.warning("Docker client is not initialized, events won't be collected.")
            return self.events, changed_container_ids

        now = int(time.time())

        event_generator = self.client.events(since=self._latest_event_collection_ts,
                                             until=now, decode=True)
        self._latest_event_collection_ts = now
        for event in event_generator:
            # due to [0] it might happen that the returned `event` is not a dict as expected but a string,
            #
            # [0]: https://github.com/docker/docker-py/pull/1082
            if not isinstance(event, dict):
                log.debug('Unable to parse Docker event: %s', event)
                continue

            if event.get('status') in CONFIG_RELOAD_STATUS:
                changed_container_ids.add(event.get('id'))
            self.events.append(event)
        return self.events, changed_container_ids

    @classmethod
    def get_gateway(cls, proc_prefix=""):
        procfs_route = os.path.join("/", proc_prefix, cls.DEFAULT_PROCFS_GW_PATH)

        try:
            with open(procfs_route) as f:
                for line in f.readlines():
                    fields = line.strip().split()
                    if fields[1] == '00000000':
                        return socket.inet_ntoa(struct.pack('<L', int(fields[2], 16)))
        except IOError, e:
            log.error('Unable to open {}: %s'.format(procfs_route), e)

        return None

    def get_hostname(self, use_default_gw=True, should_resolve=False):
        '''
        Return the `Name` param from `docker info` to use as the hostname
        Falls back to the default route.
        '''
        # return or raise
        is_resolvable = lambda host: socket.gethostbyname(host)

        if self.hostname is not None:
            # Use cache
            try:
                if not should_resolve or is_resolvable(self.hostname):
                    return self.hostname
            except Exception:
                log.debug("Couldn't resolve cached hostname %s, triggering new hostname detection." % self.hostname)

        if self._default_gateway is not None and use_default_gw:
            return self._default_gateway

        try:
            self.hostname = self.client.info().get("Name")
            if not should_resolve or is_resolvable(self.hostname):
                return self.hostname
        except Exception as e:
            log.debug("Unable to retrieve hostname using docker API, %s", str(e))
            if not use_default_gw:
                return None

        log.warning("Unable to find docker host hostname. Trying default route")
        self._default_gateway = DockerUtil.get_gateway()

        return self._default_gateway

    def get_host_tags(self):
        tags = []
        if not self.client:
            log.warning("Docker client is not initialized, host tags will be missing.")
            return tags
        version = self.client.version()
        if version and 'Version' in version:
            tags.append('docker_version:%s' % version['Version'])
        else:
            log.debug("Could not determine Docker version")

        if self.is_swarm():
            tags.append('docker_swarm:active')

        return tags

    def set_docker_settings(self, init_config, instance):
        """Update docker settings"""
        self._docker_root = init_config.get('docker_root', '/')
        self.settings = {
            "version": init_config.get('api_version', DEFAULT_VERSION),
            "base_url": instance.get("url", ''),
            "timeout": int(init_config.get('timeout', DEFAULT_TIMEOUT)),
        }

        if init_config.get('tls', False):
            client_cert_path = init_config.get('tls_client_cert')
            client_key_path = init_config.get('tls_client_key')
            cacert = init_config.get('tls_cacert')
            verify = init_config.get('tls_verify', False)

            client_cert = None
            if client_cert_path is not None and client_key_path is not None:
                client_cert = (client_cert_path, client_key_path)

            verify = cacert if cacert is not None else verify
            tls_config = tls.TLSConfig(client_cert=client_cert, verify=verify)
            self.settings["tls"] = tls_config

    def get_mountpoints(self, cgroup_metrics):
        mountpoints = {}
        for metric in cgroup_metrics:
            try:
                mountpoints[metric["cgroup"]] = self.find_cgroup(metric["cgroup"])
            except CGroupException as e:
                log.exception("Unable to find cgroup: %s", e)

        if not len(mountpoints):
            raise CGroupException("No cgroups were found!")

        return mountpoints

    def find_cgroup(self, hierarchy):
        """Find the mount point for a specified cgroup hierarchy.

        Works with old style and new style mounts.

        An example of what the output of /proc/mounts looks like:

            cgroup /sys/fs/cgroup/cpuset cgroup rw,relatime,cpuset 0 0
            cgroup /sys/fs/cgroup/cpu cgroup rw,relatime,cpu 0 0
            cgroup /sys/fs/cgroup/cpuacct cgroup rw,relatime,cpuacct 0 0
            cgroup /sys/fs/cgroup/memory cgroup rw,relatime,memory 0 0
            cgroup /sys/fs/cgroup/devices cgroup rw,relatime,devices 0 0
            cgroup /sys/fs/cgroup/freezer cgroup rw,relatime,freezer 0 0
            cgroup /sys/fs/cgroup/blkio cgroup rw,relatime,blkio 0 0
            cgroup /sys/fs/cgroup/perf_event cgroup rw,relatime,perf_event 0 0
            cgroup /sys/fs/cgroup/hugetlb cgroup rw,relatime,hugetlb 0 0
        """
        with open(os.path.join(self._docker_root, "/proc/mounts"), 'r') as fp:
            mounts = map(lambda x: x.split(), fp.read().splitlines())
        cgroup_mounts = filter(lambda x: x[2] == "cgroup", mounts)
        if len(cgroup_mounts) == 0:
            raise Exception(
                "Can't find mounted cgroups. If you run the Agent inside a container,"
                " please refer to the documentation.")
        # Old cgroup style
        if len(cgroup_mounts) == 1:
            return os.path.join(self._docker_root, cgroup_mounts[0][1])

        candidate = None
        for _, mountpoint, _, opts, _, _ in cgroup_mounts:
            if any(opt == hierarchy for opt in opts.split(',')) and os.path.exists(mountpoint):
                if mountpoint.startswith("/host/"):
                    return os.path.join(self._docker_root, mountpoint)
                candidate = mountpoint

        if candidate is not None:
            return os.path.join(self._docker_root, candidate)
        raise CGroupException("Can't find mounted %s cgroups." % hierarchy)

    def build_filters(self):
        """Build sets of include/exclude patters and of all filtered tag names based on these"""
        # The reasoning is to check exclude first, so we can skip if there is no exclude
        if not self._exclude:
            return

        filtered_tag_names = []
        exclude_patterns = []
        include_patterns = []

        # Compile regex
        for rule in self._exclude:
            exclude_patterns.append(re.compile(rule))
            filtered_tag_names.append(rule.split(':')[0])
        for rule in self._include:
            include_patterns.append(re.compile(rule))
            filtered_tag_names.append(rule.split(':')[0])

        self._exclude_patterns, self._include_patterns = set(exclude_patterns), set(include_patterns)
        self._filtered_tag_names = set(filtered_tag_names)

    @property
    def filtered_tag_names(self):
        return list(self._filtered_tag_names)

    def are_tags_filtered(self, tags):
        if not self.filtering_enabled:
            return False
        if self._tags_match_patterns(tags, self._exclude_patterns):
            if self._tags_match_patterns(tags, self._include_patterns):
                return False
            return True
        return False

    def _tags_match_patterns(self, tags, filters):
        for rule in filters:
            for tag in tags:
                if re.match(rule, tag):
                    return True
        return False

    @classmethod
    def _parse_subsystem(cls, line):
        """
        Parse cgroup path.
        - If the path is a slice (see https://access.redhat.com/documentation/en-US/Red_Hat_Enterprise_Linux/7/html/Resource_Management_Guide/sec-Default_Cgroup_Hierarchies.html)
          we return the path as-is (we still strip out any leading '/')
        - If 'docker' is in the path, it can be there once or twice:
          /docker/$CONTAINER_ID
          /docker/$USER_DOCKER_CID/docker/$CONTAINER_ID
          so we pick the last one.
        In /host/sys/fs/cgroup/$CGROUP_FOLDER/ cgroup/container IDs can be at the root
        or in a docker folder, so if we find 'docker/' in the path we don't strip it away.
        """
        if '.slice' in line[2]:
            return line[2].lstrip('/')
        i = line[2].rfind('docker')
        if i != -1:  # rfind returns -1 if docker is not found
            return line[2][i:]
        elif line[2][0] == '/':
            return line[2][1:]
        else:
            return line[2]

    @classmethod
    def find_cgroup_from_proc(cls, mountpoints, pid, subsys, docker_root='/'):
        proc_path = os.path.join(docker_root, 'proc', str(pid), 'cgroup')
        with open(proc_path, 'r') as fp:
            lines = map(lambda x: x.split(':'), fp.read().splitlines())
            subsystems = dict(zip(map(lambda x: x[1], lines), map(cls._parse_subsystem, lines)))

        if subsys not in subsystems and subsys == 'cpuacct':
            for form in "{},cpu", "cpu,{}":
                _subsys = form.format(subsys)
                if _subsys in subsystems:
                    subsys = _subsys
                    break

        # In Ubuntu Xenial, we've encountered containers with no `cpu`
        # cgroup in /proc/<pid>/cgroup
        if subsys == 'cpu' and subsys not in subsystems:
            for sub, mountpoint in subsystems.iteritems():
                if 'cpuacct' in sub:
                    subsystems['cpu'] = mountpoint
                    break

        if subsys in subsystems:
            for mountpoint in mountpoints.itervalues():
                stat_file_path = os.path.join(mountpoint, subsystems[subsys])
                if subsys == mountpoint.split('/')[-1] and os.path.exists(stat_file_path):
                    return os.path.join(stat_file_path, '%(file)s')

                # CentOS7 will report `cpu,cpuacct` and then have the path on
                # `cpuacct,cpu`
                if 'cpuacct' in mountpoint and ('cpuacct' in subsys or 'cpu' in subsys):
                    flipkey = subsys.split(',')
                    flipkey = "{},{}".format(flipkey[1], flipkey[0]) if len(flipkey) > 1 else flipkey[0]
                    mountpoint = os.path.join(os.path.split(mountpoint)[0], flipkey)
                    stat_file_path = os.path.join(mountpoint, subsystems[subsys])
                    if os.path.exists(stat_file_path):
                        return os.path.join(stat_file_path, '%(file)s')

        raise MountException("Cannot find Docker '%s' cgroup directory. Be sure your system is supported." % subsys)

    @classmethod
    def find_cgroup_filename_pattern(cls, mountpoints, container_id):
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

    def extract_container_tags(self, co):
        """
        Retrives docker_image, image_name and image_tag tags as a list for a
        container. If the container or image is invalid, will gracefully
        return an empty list
        """
        tags = []
        docker_image = self.image_name_extractor(co)
        image_name_array = self.image_tag_extractor(co, 0)
        image_tag_array = self.image_tag_extractor(co, 1)

        if docker_image:
            tags.append('docker_image:%s' % docker_image)
        if image_name_array and len(image_name_array) > 0:
            tags.append('image_name:%s' % image_name_array[0])
        if image_tag_array and len(image_tag_array) > 0:
            tags.append('image_tag:%s' % image_tag_array[0])
        return tags

    def image_tag_extractor(self, entity, key):
        name = self.image_name_extractor(entity)
        if name is not None and len(name):
            split = name.split(":")
            if len(split) <= key:
                return None
            elif len(split) > 2:
                # if the repo is in the image name and has the form 'docker.clearbit:5000'
                # the split will be like [repo_url, repo_port/image_name, image_tag]. Let's avoid that
                split = [':'.join(split[:-1]), split[-1]]
            return [split[key]]
        # Entity is an image. TODO: deprecate?
        elif entity.get('RepoTags'):
            splits = [el.split(":") for el in entity["RepoTags"]]
            tags = set()
            for split in splits:
                if len(split) > 2:
                    split = [':'.join(split[:-1]), split[-1]]
                if len(split) > key:
                    tags.add(split[key])
            if len(tags) > 0:
                return list(tags)
        elif entity.get('RepoDigests'):
            # the human-readable tag is not mentioned in RepoDigests, only the image name
            if key != 0:
                return None
            split = entity['RepoDigests'][0].split('@')
            if len(split) > 1:
                return [split[key]]

        return None

    def image_name_extractor(self, co):
        """
        Returns the image name for a container, either directly from the
        container's Image property or by inspecting the image entity if
        the reference is its sha256 sum and not its name.
        Result is cached for performance, no invalidation planned as image
        churn is low on typical hosts.
        """
        return self.image_name_resolver(co.get('Image', ''))

    def image_name_resolver(self, image):
        if image.startswith('sha256:'):
            # Some orchestrators setup containers with image checksum instead of image name
            try:
                if image in self._image_sha_to_name_mapping:
                    return self._image_sha_to_name_mapping[image]
                else:
                    try:
                        image_spec = self.client.inspect_image(image)
                    except NotFound:
                        return
                    try:
                        name = image_spec['RepoTags'][0]
                        self._image_sha_to_name_mapping[image] = name
                        return name
                    except (LookupError, TypeError) as e:
                        log.debug("Failed finding image name in RepoTag, trying RepoDigests: %s", e)
                    try:
                        name = image_spec['RepoDigests'][0]
                        name = name.split('@')[0]   # Last resort, we get the name with no tag
                        self._image_sha_to_name_mapping[image] = name
                        return name
                    except (LookupError, TypeError) as e:
                        log.debug("Failed finding image name in RepoTag and RepoDigests: %s", e)
            except Exception as ex:
                log.error("Exception getting docker image name: %s" % str(ex))
        else:
            return image

    @classmethod
    def container_name_extractor(cls, co):
        names = co.get('Names', [])
        if names is not None:
            # we sort the list to make sure that a docker API update introducing
            # new names with a single "/" won't make us report dups.
            names = sorted(names)
            for name in names:
                # the leading "/" is legit, if there's another one it means the name is actually an alias
                if name.count('/') <= 1:
                    return [str(name).lstrip('/')]
        return [co.get('Id')[:12]]

    @classmethod
    def get_container_network_mapping(cls, container):
        """Matches /proc/$PID/net/route and docker inspect to map interface names to docker network name.
        Raises an exception on error (dict lookup or file parsing), to be caught by the using method"""

        try:
            proc_net_route_file = os.path.join(container['_proc_root'], 'net/route')

            docker_gateways = {}
            for netname, netconf in container['NetworkSettings']['Networks'].iteritems():

                if netname == 'host' or netconf.get(u'Gateway') == '':
                    log.debug("Empty network gateway, container %s is in network host mode, "
                        "its network metrics are for the whole host." % container['Id'][:12])
                    return {'eth0': 'bridge'}

                docker_gateways[netname] = struct.unpack('<L', socket.inet_aton(netconf.get(u'Gateway')))[0]

            mapping = {}
            with open(proc_net_route_file, 'r') as fp:
                lines = fp.readlines()
                for l in lines[1:]:
                    cols = l.split()
                    if cols[1] == '00000000':
                        continue
                    destination = int(cols[1], 16)
                    mask = int(cols[7], 16)
                    for net, gw in docker_gateways.iteritems():
                        if gw & mask == destination:
                            mapping[cols[0]] = net
                return mapping
        except KeyError as e:
            log.exception("Missing container key: %s", e)
            raise ValueError("Invalid container dict")

    def inspect_container(self, co_id):
        """
        Requests docker inspect for one container. This is a costly operation!
        :param co_id: container id
        :return: dict from docker-py
        """
        return self.client.inspect_container(co_id)

    @classmethod
    def _drop(cls):
        if cls in cls._instances:
            del cls._instances[cls]
