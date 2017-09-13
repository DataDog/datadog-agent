# (C) Datadog, Inc. 2017
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

import os
import requests

from .baseutil import BaseUtil

CHRONOS_JOB_NAME = "CHRONOS_JOB_NAME"
CHRONOS_JOB_OWNER = "CHRONOS_JOB_OWNER"
MARATHON_APP_ID = "MARATHON_APP_ID"
MESOS_TASK_ID = "MESOS_TASK_ID"

MESOS_AGENT_IP_ENV = ["LIBPROCESS_IP", "HOST", "HOSTNAME"]
MESOS_AGENT_HTTP_PORT = 5051
DCOS_AGENT_HTTP_PORT = 61001
DOCS_AGENT_HTTPS_PORT = 61002

MESOS_VERSION_URL_TEMPLATE = "http://%s:%d/version"
DCOS_HEALTH_URL_TEMPLATE = "http://%s:%d/system/health/v1"


class MesosUtil(BaseUtil):
    def __init__(self):
        BaseUtil.__init__(self)
        self.needs_inspect_config = True
        self.mesos_agent_url, self.dcos_agent_url = self._detect_agents()

    def _get_cacheable_tags(self, cid, co=None):
        tags = []
        envvars = co.get('Config', {}).get('Env', {})

        for var in envvars:
            if var.startswith(MARATHON_APP_ID):
                tags.append('marathon_app:%s' % var[len(MARATHON_APP_ID) + 1:])
            elif var.startswith(CHRONOS_JOB_NAME) and len(var) > len(CHRONOS_JOB_NAME) + 1:
                tags.append('chronos_job:%s' % var[len(CHRONOS_JOB_NAME) + 1:])
            elif var.startswith(CHRONOS_JOB_OWNER) and len(var) > len(CHRONOS_JOB_OWNER) + 1:
                tags.append('chronos_job_owner:%s' % var[len(CHRONOS_JOB_OWNER) + 1:])
            ## Disabled for now because of high cardinality (~container card.)
            #elif var.startswith(MESOS_TASK_ID):
            #    tags.append('mesos_task:%s' % var[len(MESOS_TASK_ID) + 1:])

        return tags

    @staticmethod
    def mesos_agent_validation(r):
        return "version" in r.json()

    @staticmethod
    def dcos_agent_validation(r):
        return "dcos_version" in r.json()

    def _detect_agents(self):
        """
        The Mesos agent runs on every node and listens to http port 5051
        See https://mesos.apache.org/documentation/latest/endpoints/
        We'll use the unauthenticated endpoint /version

        The DCOS agent runs on every node and listens to ports 61001 or 61002
        See https://dcos.io/docs/1.9/api/agent-routes/
        We'll use the unauthenticated endpoint /system/health/v1
        """
        mesos_urls = []
        dcos_urls = []
        for var in MESOS_AGENT_IP_ENV:
            if var in os.environ:
                mesos_urls.append(MESOS_VERSION_URL_TEMPLATE %
                                  (os.environ.get(var), MESOS_AGENT_HTTP_PORT))
                dcos_urls.append(DCOS_HEALTH_URL_TEMPLATE %
                                 (os.environ.get(var), DCOS_AGENT_HTTP_PORT))
                dcos_urls.append(DCOS_HEALTH_URL_TEMPLATE %
                                 (os.environ.get(var), DOCS_AGENT_HTTPS_PORT))
        # Try network gateway last
        gw = self.docker_util.get_gateway()
        if gw:
            mesos_urls.append(MESOS_VERSION_URL_TEMPLATE % (gw, MESOS_AGENT_HTTP_PORT))
            dcos_urls.append(DCOS_HEALTH_URL_TEMPLATE % (gw, DCOS_AGENT_HTTP_PORT))
            dcos_urls.append(DCOS_HEALTH_URL_TEMPLATE % (gw, DOCS_AGENT_HTTPS_PORT))

        mesos_url = self._try_urls(mesos_urls, validation_lambda=MesosUtil.mesos_agent_validation)
        if mesos_url:
            self.log.debug("Found Mesos agent at " + mesos_url)
        else:
            self.log.debug("Could not find Mesos agent at urls " + str(mesos_urls))
        dcos_url = self._try_urls(dcos_urls, validation_lambda=MesosUtil.dcos_agent_validation)
        if dcos_url:
            self.log.debug("Found DCOS agent at " + dcos_url)
        else:
            self.log.debug("Could not find DCOS agent at urls " + str(dcos_urls))

        return (mesos_url, dcos_url)

    @staticmethod
    def is_detected():
        return MESOS_TASK_ID in os.environ

    def get_host_tags(self):
        tags = []
        if self.mesos_agent_url:
            try:
                resp = requests.get(self.mesos_agent_url, timeout=1).json()
                if "version" in resp:
                    tags.append('mesos_version:%s' % resp.get("version"))
            except Exception as e:
                self.log.debug("Error getting Mesos version: %s" % str(e))

        if self.dcos_agent_url:
            try:
                resp = requests.get(self.dcos_agent_url, timeout=1).json()
                if "dcos_version" in resp:
                    tags.append('dcos_version:%s' % resp.get("dcos_version"))
            except Exception as e:
                self.log.debug("Error getting DCOS version: %s" % str(e))

        return tags
