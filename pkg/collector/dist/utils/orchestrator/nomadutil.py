# (C) Datadog, Inc. 2017
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

import os
import requests

from .baseutil import BaseUtil

NOMAD_TASK_NAME = 'NOMAD_TASK_NAME'
NOMAD_JOB_NAME = 'NOMAD_JOB_NAME'
NOMAD_ALLOC_NAME = 'NOMAD_ALLOC_NAME'
NOMAD_ALLOC_ID = 'NOMAD_ALLOC_ID'

NOMAD_AGENT_URL = "http://%s:4646/v1/agent/self"


class NomadUtil(BaseUtil):
    def __init__(self):
        BaseUtil.__init__(self)
        self.needs_inspect_config = True
        self.agent_url = self._detect_agent()

    def _get_cacheable_tags(self, cid, co=None):
        tags = []
        envvars = co.get('Config', {}).get('Env', {})
        for var in envvars:
            if var.startswith(NOMAD_TASK_NAME):
                tags.append('nomad_task:%s' % var[len(NOMAD_TASK_NAME) + 1:])
            elif var.startswith(NOMAD_JOB_NAME):
                tags.append('nomad_job:%s' % var[len(NOMAD_JOB_NAME) + 1:])
            elif var.startswith(NOMAD_ALLOC_NAME):
                try:
                    start = var.index('.', len(NOMAD_ALLOC_NAME)) + 1
                    end = var.index('[')
                    if end <= start:
                        raise ValueError("Error extracting group from %s, check format" % var)
                    tags.append('nomad_group:%s' % var[start:end])
                except ValueError:
                    pass
        return tags

    @staticmethod
    def is_detected():
        return NOMAD_ALLOC_ID in os.environ

    @staticmethod
    def nomad_agent_validation(r):
        return "Version" in r.json().get('config', {})

    def _detect_agent(self):
        """
        The Nomad agent runs on every node and listens to http port 4646
        See https://www.nomadproject.io/docs/http/agent-self.html
        We'll use the unauthenticated endpoint /v1/agent/self

        We don't have any envvars or downwards API to help us, so we try
        default gw (network=bridge) and localhost (network=host), but can't
        auto-detect more complicated cases
        We need the agent to listen to 0.0.0.0, which is not the case on a devnode
        """
        urls = []

        gw = self.docker_util.get_gateway()
        if gw:
            urls.append(NOMAD_AGENT_URL % gw)
        urls.append(NOMAD_AGENT_URL % "127.0.0.1")

        nomad_url = self._try_urls(urls, validation_lambda=NomadUtil.nomad_agent_validation)
        if nomad_url:
            self.log.debug("Found Nomad agent at " + nomad_url)
        else:
            self.log.debug("Could not find Nomad agent at urls " + str(urls))

        return nomad_url

    def get_host_tags(self):
        tags = []
        if self.agent_url:
            try:
                resp = requests.get(self.agent_url, timeout=1).json().get('config', {})
                if "Version" in resp:
                    tags.append('nomad_version:%s' % resp.get("Version"))
                if "Region" in resp:
                    tags.append('nomad_region:%s' % resp.get("Region"))
                if "Datacenter" in resp:
                    tags.append('nomad_datacenter:%s' % resp.get("Datacenter"))
            except Exception as e:
                self.log.debug("Error getting Nomad version: %s" % str(e))

        return tags
