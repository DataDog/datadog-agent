# (C) Datadog, Inc. 2018
# All rights reserved
# Licensed under a 3-clause BSD style license (see LICENSE)

import datetime

from datadog_checks.checks import AgentCheck

from datadog_checks.utils.containers import hash_mutable

from datadog_checks.config import _is_affirmative

from . import metrics as aci_metrics
from .capacity import Capacity
from .tenant import Tenant
from .fabric import Fabric
from .tags import CiscoTags
from .api import Api

try:
    # Agent >= 6.0: the check pushes tags invoking `set_external_tags`
    from datadog_agent import set_external_tags
except ImportError:
    # Agent < 6.0: the Agent pulls tags invoking `CiscoACICheck.get_external_host_tags`
    set_external_tags = None

SOURCE_TYPE = 'cisco_aci'

SERVICE_CHECK_NAME = 'cisco_aci.can_connect'


class CiscoACICheck(AgentCheck):

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        self.tenant_metrics = aci_metrics.make_tenant_metrics()
        self.last_events_ts = {}
        self.external_host_tags = {}
        self._api_cache = {}
        self.check_tags = ['cisco']
        self.tagger = CiscoTags(self)

    def check(self, instance):
        self.log.info("Starting Cisco Check")
        start = datetime.datetime.now()
        aci_url = instance.get('aci_url')
        aci_urls = instance.get('aci_urls', [])
        if aci_url:
            aci_urls.append(aci_url)

        if len(aci_urls) == 0:
            raise Exception("The Cisco ACI check requires at least one url")

        username = instance['username']
        pwd = instance['pwd']
        instance_hash = hash_mutable(instance)

        timeout = instance.get('timeout', 15)
        ssl_verify = _is_affirmative(instance.get('ssl_verify', True))

        if instance_hash in self._api_cache:
            api = self._api_cache.get(instance_hash)
        else:
            api = Api(aci_urls, username, pwd, verify=ssl_verify, timeout=timeout, log=self.log)
            self._api_cache[instance_hash] = api

        service_check_tags = []
        for url in aci_urls:
            service_check_tags.append("url:{}".format(url))
        service_check_tags.extend(self.check_tags)
        service_check_tags.extend(instance.get('tags', []))

        try:
            api.login()
        except Exception as e:
            self.log.error("Cannot login to the Cisco ACI: {}".format(e))
            self.service_check(SERVICE_CHECK_NAME,
                               AgentCheck.CRITICAL,
                               message="aci login returned a status of {}".format(e),
                               tags=service_check_tags)
            raise

        self.tagger.api = api

        try:
            tenant = Tenant(self, api, instance, instance_hash)
            tenant.collect()
        except Exception as e:
            self.log.error('tenant collection failed: {}'.format(e))
            self.service_check(SERVICE_CHECK_NAME,
                               AgentCheck.CRITICAL,
                               message="aci tenant operations failed, returning a status of {}".format(e),
                               tags=service_check_tags)
            api.close()
            raise

        try:
            fabric = Fabric(self, api, instance)
            fabric.collect()
        except Exception as e:
            self.log.error('fabric collection failed: {}'.format(e))
            self.service_check(SERVICE_CHECK_NAME,
                               AgentCheck.CRITICAL,
                               message="aci fabric operations failed, returning a status of {}".format(e),
                               tags=service_check_tags)
            api.close()
            raise

        try:
            capacity = Capacity(self, api, instance)
            capacity.collect()
        except Exception as e:
            self.log.error('capacity collection failed: {}'.format(e))
            self.service_check(SERVICE_CHECK_NAME,
                               AgentCheck.CRITICAL,
                               message="aci capacity operations failed, returning a status of {}".format(e),
                               tags=service_check_tags)
            api.close()
            raise

        self.service_check(SERVICE_CHECK_NAME,
                           AgentCheck.OK,
                           tags=service_check_tags)

        if set_external_tags:
            set_external_tags(self.get_external_host_tags())

        api.close()
        end = datetime.datetime.now()
        log_line = "finished running Cisco Check"
        if _is_affirmative(instance.get('report_timing', False)):
            log_line += ", took {}".format(end - start)
        self.log.info(log_line)

    def submit_metrics(self, metrics, tags, instance={}, obj_type="gauge", hostname=None):
        user_tags = instance.get('tags', [])
        for mname, mval in metrics.iteritems():
            tags_to_send = []
            if mval:
                if hostname:
                    tags_to_send += self.check_tags
                tags_to_send += user_tags + tags
                if obj_type == "gauge":
                    self.gauge(mname, float(mval), tags=tags_to_send, hostname=hostname)
                elif obj_type == "rate":
                    self.rate(mname, float(mval), tags=tags_to_send, hostname=hostname)
                else:
                    log_line = "Trying to submit metric: {} with unknown type: {}"
                    log_line = log_line.format(mname, obj_type)
                    self.log.debug(log_line)

    def get_external_host_tags(self):
        external_host_tags = []
        for hostname, tags in self.external_host_tags.iteritems():
            host_tags = tags + self.check_tags
            external_host_tags.append((hostname, {SOURCE_TYPE: host_tags}))
        return external_host_tags
