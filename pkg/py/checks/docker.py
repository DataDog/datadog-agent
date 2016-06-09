# stdlib
import urllib2
import urllib
import httplib
import socket
import os
import re
import time
from urlparse import urlsplit
from util import json
from collections import defaultdict

# project
from checks import AgentCheck
from config import _is_affirmative

EVENT_TYPE = SOURCE_TYPE_NAME = 'docker'

CGROUP_METRICS = [
    {
        "cgroup": "memory",
        "file": "memory.stat",
        "metrics": {
            # Default metrics
            "cache": ("docker.mem.cache", "gauge", True),
            "rss": ("docker.mem.rss", "gauge", True),
            "swap": ("docker.mem.swap", "gauge", True),
            # Optional metrics
            "active_anon": ("docker.mem.active_anon", "gauge", False),
            "active_file": ("docker.mem.active_file", "gauge", False),
            "inactive_anon": ("docker.mem.inactive_anon", "gauge", False),
            "inactive_file": ("docker.mem.inactive_file", "gauge", False),
            "mapped_file": ("docker.mem.mapped_file", "gauge", False),
            "pgfault": ("docker.mem.pgfault", "rate", False),
            "pgmajfault": ("docker.mem.pgmajfault", "rate", False),
            "pgpgin": ("docker.mem.pgpgin", "rate", False),
            "pgpgout": ("docker.mem.pgpgout", "rate", False),
            "unevictable": ("docker.mem.unevictable", "gauge", False),
        }
    },
    {
        "cgroup": "cpuacct",
        "file": "cpuacct.stat",
        "metrics": {
            "user": ("docker.cpu.user", "rate", True),
            "system": ("docker.cpu.system", "rate", True),
        },
    },
]

DOCKER_METRICS = {
    "SizeRw": ("docker.disk.size", "gauge"),
}

DOCKER_TAGS = [
    "Command",
    "Image",
]

NEW_TAGS_MAP = {
    "name": "container_name",
    "image": "docker_image",
    "command": "container_command",
}

DEFAULT_SOCKET_TIMEOUT = 5

class DockerJSONDecodeError(Exception):
    """ Raised when there is trouble parsing the API response sent by Docker Remote API """
    pass

class UnixHTTPConnection(httplib.HTTPConnection):
    """Class used in conjuction with UnixSocketHandler to make urllib2
    compatible with Unix sockets."""

    socket_timeout = DEFAULT_SOCKET_TIMEOUT

    def __init__(self, unix_socket):
        self._unix_socket = unix_socket

    def connect(self):
        sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        sock.connect(self._unix_socket)
        sock.settimeout(self.socket_timeout)
        self.sock = sock

    def __call__(self, *args, **kwargs):
        httplib.HTTPConnection.__init__(self, *args, **kwargs)
        return self


class UnixSocketHandler(urllib2.AbstractHTTPHandler):
    """Class that makes Unix sockets work with urllib2 without any additional
    dependencies."""
    def unix_open(self, req):
        full_path = "%s%s" % urlsplit(req.get_full_url())[1:3]
        path = os.path.sep
        for part in full_path.split("/"):
            path = os.path.join(path, part)
            if not os.path.exists(path):
                break
            unix_socket = path
        # add a host or else urllib2 complains
        url = req.get_full_url().replace(unix_socket, "/localhost")
        new_req = urllib2.Request(url, req.get_data(), dict(req.header_items()))
        new_req.timeout = req.timeout
        return self.do_open(UnixHTTPConnection(unix_socket), new_req)

    unix_request = urllib2.AbstractHTTPHandler.do_request_


class Docker(AgentCheck):
    """Collect metrics and events from Docker API and cgroups"""

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)

        # Initialize a HTTP opener with Unix socket support
        socket_timeout = int(init_config.get('socket_timeout', 0)) or DEFAULT_SOCKET_TIMEOUT
        UnixHTTPConnection.socket_timeout = socket_timeout
        self.url_opener = urllib2.build_opener(UnixSocketHandler())

        # Locate cgroups directories
        self._mountpoints = {}
        self._cgroup_filename_pattern = None
        docker_root = init_config.get('docker_root', '/')
        for metric in CGROUP_METRICS:
            self._mountpoints[metric["cgroup"]] = self._find_cgroup(metric["cgroup"], docker_root)

        self._last_event_collection_ts = defaultdict(lambda: None)

    def check(self, instance):
        # Report image metrics
        self.warning('Using the "docker" check is deprecated and will be removed'
        ' in a future version of the agent. Please use the "docker_daemon" one instead')
        if _is_affirmative(instance.get('collect_images_stats', True)):
            self._count_images(instance)

        # Get the list of containers and the index of their names
        containers, ids_to_names = self._get_and_count_containers(instance)

        # Report container metrics from cgroups
        skipped_container_ids = self._report_containers_metrics(containers, instance)

        # Send events from Docker API
        if _is_affirmative(instance.get('collect_events', True)):
            self._process_events(instance, ids_to_names, skipped_container_ids)


    # Containers

    def _count_images(self, instance):
        # It's not an important metric, keep going if it fails
        try:
            tags = instance.get("tags", [])
            active_images = len(self._get_images(instance, get_all=False))
            all_images = len(self._get_images(instance, get_all=True))

            self.gauge("docker.images.available", active_images, tags=tags)
            self.gauge("docker.images.intermediate", (all_images - active_images), tags=tags)
        except Exception, e:
            self.warning("Failed to count Docker images. Exception: {0}".format(e))

    def _get_and_count_containers(self, instance):
        tags = instance.get("tags", [])
        with_size = _is_affirmative(instance.get('collect_container_size', False))

        service_check_name = 'docker.service_up'
        try:
            running_containers = self._get_containers(instance, with_size=with_size)
            all_containers = self._get_containers(instance, get_all=True)
        except (socket.timeout, urllib2.URLError), e:
            self.service_check(service_check_name, AgentCheck.CRITICAL,
                message="Unable to list Docker containers: {0}".format(e))
            raise Exception("Failed to collect the list of containers. Exception: {0}".format(e))
        self.service_check(service_check_name, AgentCheck.OK)

        running_containers_ids = set([container['Id'] for container in running_containers])

        for container in all_containers:
            container_tags = list(tags)
            for key in DOCKER_TAGS:
                tag = self._make_tag(key, container[key], instance)
                if tag:
                    container_tags.append(tag)
            if container['Id'] in running_containers_ids:
                self.set("docker.containers.running", container['Id'], tags=container_tags)
            else:
                self.set("docker.containers.stopped", container['Id'], tags=container_tags)

        # The index of the names is used to generate and format events
        ids_to_names = {}
        for container in all_containers:
            ids_to_names[container['Id']] = self._get_container_name(container)

        return running_containers, ids_to_names

    def _get_container_name(self, container):
        # Use either the first container name or the container ID to name the container in our events
        if container.get('Names', []):
            return container['Names'][0].lstrip("/")
        return container['Id'][:11]

    def _prepare_filters(self, instance):
        # The reasoning is to check exclude first, so we can skip if there is no exclude
        if not instance.get("exclude"):
            return False

        # Compile regex
        instance["exclude_patterns"] = [re.compile(rule) for rule in instance.get("exclude", [])]
        instance["include_patterns"] = [re.compile(rule) for rule in instance.get("include", [])]

        return True

    def _is_container_excluded(self, instance, tags):
        if self._tags_match_patterns(tags, instance.get("exclude_patterns")):
            if self._tags_match_patterns(tags, instance.get("include_patterns")):
                return False
            return True
        return False

    def _tags_match_patterns(self, tags, filters):
        for rule in filters:
            for tag in tags:
                if re.match(rule, tag):
                    return True
        return False

    def _report_containers_metrics(self, containers, instance):
        skipped_container_ids = []
        collect_uncommon_metrics = _is_affirmative(instance.get("collect_all_metrics", False))
        tags = instance.get("tags", [])

        # Pre-compile regex to include/exclude containers
        use_filters = self._prepare_filters(instance)

        for container in containers:
            container_tags = list(tags)
            for name in container["Names"]:
                container_tags.append(self._make_tag("name", name.lstrip("/"), instance))
            for key in DOCKER_TAGS:
                tag = self._make_tag(key, container[key], instance)
                if tag:
                    container_tags.append(tag)

            # Check if the container is included/excluded via its tags
            if use_filters and self._is_container_excluded(instance, container_tags):
                skipped_container_ids.append(container['Id'])
                continue

            for key, (dd_key, metric_type) in DOCKER_METRICS.iteritems():
                if key in container:
                    getattr(self, metric_type)(dd_key, int(container[key]), tags=container_tags)
            for cgroup in CGROUP_METRICS:
                stat_file = self._get_cgroup_file(cgroup["cgroup"], container['Id'], cgroup['file'])
                stats = self._parse_cgroup_file(stat_file)
                if stats:
                    for key, (dd_key, metric_type, common_metric) in cgroup['metrics'].iteritems():
                        if key in stats and (common_metric or collect_uncommon_metrics):
                            getattr(self, metric_type)(dd_key, int(stats[key]), tags=container_tags)
        if use_filters:
            self.log.debug("List of excluded containers: {0}".format(skipped_container_ids))

        return skipped_container_ids

    def _make_tag(self, key, value, instance):
        tag_name = key.lower()
        if tag_name == "command" and not instance.get("tag_by_command", False):
            return None
        if instance.get("new_tag_names", False):
            tag_name = self._new_tags_conversion(tag_name)

        return "%s:%s" % (tag_name, value.strip())

    def _new_tags_conversion(self, tag):
        # Prefix tags to avoid conflict with AWS tags
        return NEW_TAGS_MAP.get(tag, tag)


    # Events

    def _process_events(self, instance, ids_to_names, skipped_container_ids):
        try:
            api_events = self._get_events(instance)
            aggregated_events = self._pre_aggregate_events(api_events, skipped_container_ids)
            events = self._format_events(aggregated_events, ids_to_names)
            self._report_events(events)
        except (socket.timeout, urllib2.URLError):
            self.warning('Timeout during socket connection. Events will be missing.')

    def _pre_aggregate_events(self, api_events, skipped_container_ids):
        # Aggregate events, one per image. Put newer events first.
        events = defaultdict(list)
        for event in api_events:
            # Skip events related to filtered containers
            if event['id'] in skipped_container_ids:
                self.log.debug("Excluded event: container {0} status changed to {1}".format(
                    event['id'], event['status']))
                continue
            # Known bug: from may be missing
            if 'from' in event:
                events[event['from']].insert(0, event)

        return events

    def _format_events(self, aggregated_events, ids_to_names):
        events = []
        for image_name, event_group in aggregated_events.iteritems():
            max_timestamp = 0
            status = defaultdict(int)
            status_change = []
            for event in event_group:
                max_timestamp = max(max_timestamp, int(event['time']))
                status[event['status']] += 1
                container_name = event['id'][:12]
                if event['id'] in ids_to_names:
                    container_name = "%s %s" % (container_name, ids_to_names[event['id']])
                status_change.append([container_name, event['status']])

            status_text = ", ".join(["%d %s" % (count, st) for st, count in status.iteritems()])
            msg_title = "%s %s on %s" % (image_name, status_text, self.hostname)
            msg_body = ("%%%\n"
                "{image_name} {status} on {hostname}\n"
                "```\n{status_changes}\n```\n"
                "%%%").format(
                    image_name=image_name,
                    status=status_text,
                    hostname=self.hostname,
                    status_changes="\n".join(
                        ["%s \t%s" % (change[1].upper(), change[0]) for change in status_change])
            )
            events.append({
                'timestamp': max_timestamp,
                'host': self.hostname,
                'event_type': EVENT_TYPE,
                'msg_title': msg_title,
                'msg_text': msg_body,
                'source_type_name': EVENT_TYPE,
                'event_object': 'docker:%s' % image_name,
            })

        return events

    def _report_events(self, events):
        for ev in events:
            self.log.debug("Creating event: %s" % ev['msg_title'])
            self.event(ev)


    # Docker API

    def _get_containers(self, instance, with_size=False, get_all=False):
        """Gets the list of running/all containers in Docker."""
        return self._get_json("%(url)s/containers/json" % instance, params={'size': with_size, 'all': get_all})

    def _get_images(self, instance, with_size=True, get_all=False):
        """Gets the list of images in Docker."""
        return self._get_json("%(url)s/images/json" % instance, params={'all': get_all})

    def _get_events(self, instance):
        """Get the list of events """
        now = int(time.time())
        try:
            result = self._get_json(
                "%s/events" % instance["url"],
                params={
                    "until": now,
                    "since": self._last_event_collection_ts[instance["url"]] or now - 60,
                },
                multi=True
            )
            self._last_event_collection_ts[instance["url"]] = now
            if type(result) == dict:
                result = [result]
            return result
        except DockerJSONDecodeError:
            return []

    def _get_json(self, uri, params=None, multi=False):
        """Utility method to get and parse JSON streams."""
        if params:
            uri = "%s?%s" % (uri, urllib.urlencode(params))
        self.log.debug("Connecting to Docker API at: %s" % uri)
        req = urllib2.Request(uri, None)

        try:
            request = self.url_opener.open(req)
        except urllib2.URLError, e:
            if "Errno 13" in str(e):
                raise Exception("Unable to connect to socket. dd-agent user must be part of the 'docker' group")
            raise

        response = request.read()
        response = response.replace('\n', '') # Some Docker API versions occassionally send newlines in responses
        self.log.debug('Docker API response: %s', response)
        if multi and "}{" in response: # docker api sometimes returns juxtaposed json dictionaries
            response = "[{0}]".format(response.replace("}{", "},{"))

        if not response:
            return []

        try:
            return json.loads(response)
        except Exception as e:
            self.log.error('Failed to parse Docker API response: %s', response)
            raise DockerJSONDecodeError

    # Cgroups

    def _find_cgroup_filename_pattern(self):
        if self._mountpoints:
            # We try with different cgroups so that it works even if only one is properly working
            for mountpoint in self._mountpoints.values():
                stat_file_path_lxc = os.path.join(mountpoint, "lxc")
                stat_file_path_docker = os.path.join(mountpoint, "docker")
                stat_file_path_coreos = os.path.join(mountpoint, "system.slice")

                if os.path.exists(stat_file_path_lxc):
                    return os.path.join('%(mountpoint)s/lxc/%(id)s/%(file)s')
                elif os.path.exists(stat_file_path_docker):
                    return os.path.join('%(mountpoint)s/docker/%(id)s/%(file)s')
                elif os.path.exists(stat_file_path_coreos):
                    return os.path.join('%(mountpoint)s/system.slice/docker-%(id)s.scope/%(file)s')

        raise Exception("Cannot find Docker cgroup directory. Be sure your system is supported.")

    def _get_cgroup_file(self, cgroup, container_id, filename):
        # This can't be initialized at startup because cgroups may not be mounted yet
        if not self._cgroup_filename_pattern:
            self._cgroup_filename_pattern = self._find_cgroup_filename_pattern()

        return self._cgroup_filename_pattern % (dict(
            mountpoint=self._mountpoints[cgroup],
            id=container_id,
            file=filename,
        ))

    def _find_cgroup(self, hierarchy, docker_root):
        """Finds the mount point for a specified cgroup hierarchy. Works with
        old style and new style mounts."""
        with open(os.path.join(docker_root, "/proc/mounts"), 'r') as fp:
            mounts = map(lambda x: x.split(), fp.read().splitlines())

        cgroup_mounts = filter(lambda x: x[2] == "cgroup", mounts)
        if len(cgroup_mounts) == 0:
            raise Exception("Can't find mounted cgroups. If you run the Agent inside a container,"
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


    def _parse_cgroup_file(self, stat_file):
        """Parses a cgroup pseudo file for key/values."""
        self.log.debug("Opening cgroup file: %s" % stat_file)
        try:
            with open(stat_file, 'r') as fp:
                return dict(map(lambda x: x.split(), fp.read().splitlines()))
        except IOError:
            # It is possible that the container got stopped between the API call and now
            self.log.info("Can't open %s. Metrics for this container are skipped." % stat_file)
