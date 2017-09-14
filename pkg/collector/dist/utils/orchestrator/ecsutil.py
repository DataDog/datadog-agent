# (C) Datadog, Inc. 2017
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# stdlib
import re

# 3rd
import requests

# project
from .baseutil import BaseUtil

ECS_AGENT_DEFAULT_PORT = 51678
ECS_AGENT_CONTAINER_NAME = 'ecs-agent'
ECS_AGENT_METADATA_PATH = '/v1/metadata'
ECS_AGENT_TASKS_PATH = '/v1/tasks'

AGENT_VERSION_EXP = re.compile(r'v([0-9.]+)')


class ECSUtil(BaseUtil):
    # FIXME: move the ecs detection logic from DockerUtil here?
    @staticmethod
    def is_detected():
        from utils.dockerutil import DockerUtil
        return DockerUtil().is_ecs()

    def __init__(self):
        BaseUtil.__init__(self)
        self.agent_url = self._detect_agent()
        self.ecs_tags = {}
        self._populate_ecs_tags()

    @staticmethod
    def ecs_agent_validation(r):
        return ECS_AGENT_METADATA_PATH in r.json().get("AvailableCommands", [])

    def _detect_agent(self):
        """
        The ECS agent runs on a container and listens to port 51678
        We'll test the response on / for detection
        """
        urls = []

        # Try to detect the ecs-agent container's IP (net=bridge)
        ecs_config = self.docker_util.inspect_container('ecs-agent')
        ip = ecs_config.get('NetworkSettings', {}).get('IPAddress')
        if ip:
            ports = ecs_config.get('NetworkSettings', {}).get('Ports')
            port = ports.keys()[0].split('/')[0] if ports else str(ECS_AGENT_DEFAULT_PORT)
            urls.append("http://%s:%s/" % (ip, port))

        # Try the default gateway (ecs-agent in net=host mode)
        gw = self.docker_util.get_gateway()
        if gw:
            urls.append("http://%s:%d/" % (gw, ECS_AGENT_DEFAULT_PORT))

        # Try localhost (both ecs-agent and dd-agent in host networking)
        urls.append("http://localhost:%d/" % ECS_AGENT_DEFAULT_PORT)

        url = self._try_urls(urls, validation_lambda=ECSUtil.ecs_agent_validation)
        if url:
            self.log.debug("Found ECS agent at " + url)
        else:
            self.log.debug("Could not find ECS agent at urls " + str(urls))

        return url

    def _populate_ecs_tags(self, skip_known=False):
        """
        Populate the cache of ecs tags. Can be called with skip_known=True
        If we just want to update new containers quickly (single task api call)
        (because we detected that a new task started for example)
        """
        if self.agent_url is None:
            self.log.warning("ecs-agent not found, skipping task tagging")
            return

        try:
            tasks = requests.get(self.agent_url + ECS_AGENT_TASKS_PATH, timeout=1).json()
            for task in tasks.get('Tasks', []):
                for container in task.get('Containers', []):
                    cid = container['DockerId']

                    if skip_known and cid in self.ecs_tags:
                        continue

                    tags = ['task_name:%s' % task['Family'], 'task_version:%s' % task['Version']]
                    self.ecs_tags[container['DockerId']] = tags
        except requests.exceptions.HTTPError as ex:
            self.log.warning("Unable to collect ECS task names: %s" % ex)

    def _get_cacheable_tags(self, cid, co=None):
        self._populate_ecs_tags(skip_known=True)

        if cid in self.ecs_tags:
            return self.ecs_tags[cid]
        else:
            self.log.debug("Container %s doesn't seem to be an ECS task, skipping." % cid[:12])
            self.ecs_tags[cid] = []
        return []

    # We extend the cache invalidation methods to handle the cid->task mapping cache
    def invalidate_cache(self, events):
        BaseUtil.invalidate_cache(self, events)
        try:
            for ev in events:
                if ev.get('status') == 'die' and ev.get('id') in self.ecs_tags:
                    del self.ecs_tags[ev.get('id')]
        except Exception as e:
            self.log.warning("Error when invalidating tag cache: " + str(e))

    def reset_cache(self):
        BaseUtil.reset_cache(self)
        self.ecs_tags = {}

    def get_host_tags(self):
        tags = []
        if self.agent_url:
            try:
                resp = requests.get(self.agent_url + ECS_AGENT_METADATA_PATH, timeout=1).json()
                if "Version" in resp:
                    match = AGENT_VERSION_EXP.search(resp.get("Version"))
                    if match is not None and len(match.groups()) == 1:
                        tags.append('ecs_version:%s' % match.group(1))
            except Exception as e:
                self.log.debug("Error getting ECS version: %s" % str(e))

        return tags
