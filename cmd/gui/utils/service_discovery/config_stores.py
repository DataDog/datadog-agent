# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# project
from utils.service_discovery.abstract_config_store import AbstractConfigStore
from utils.service_discovery.abstract_config_store import CONFIG_FROM_AUTOCONF, CONFIG_FROM_FILE, CONFIG_FROM_TEMPLATE, TRACE_CONFIG  # noqa imported somewhere else

from utils.service_discovery.etcd_config_store import EtcdStore
from utils.service_discovery.consul_config_store import ConsulStore
from utils.service_discovery.zookeeper_config_store import ZookeeperStore

SD_CONFIG_BACKENDS = ['etcd', 'consul', 'zk']  # noqa: used somewhere else
SD_TEMPLATE_DIR = '/datadog/check_configs'


def get_config_store(agentConfig):
    if agentConfig.get('sd_config_backend') == 'etcd':
        return EtcdStore(agentConfig)
    elif agentConfig.get('sd_config_backend') == 'consul':
        return ConsulStore(agentConfig)
    elif agentConfig.get('sd_config_backend') == 'zk':
        return ZookeeperStore(agentConfig)
    else:
        return StubStore(agentConfig)


def extract_sd_config(config):
    """Extract configuration about service discovery for the agent"""
    sd_config = {}
    if config.has_option('Main', 'sd_config_backend'):
        sd_config['sd_config_backend'] = config.get('Main', 'sd_config_backend')
    else:
        sd_config['sd_config_backend'] = None
    if config.has_option('Main', 'sd_template_dir'):
        sd_config['sd_template_dir'] = config.get(
            'Main', 'sd_template_dir')
    else:
        sd_config['sd_template_dir'] = SD_TEMPLATE_DIR
    if config.has_option('Main', 'sd_backend_host'):
        sd_config['sd_backend_host'] = config.get(
            'Main', 'sd_backend_host')
    if config.has_option('Main', 'sd_backend_port'):
        sd_config['sd_backend_port'] = config.get(
            'Main', 'sd_backend_port')
    if config.has_option('Main', 'sd_jmx_enable'):
        sd_config['sd_jmx_enable'] = config.get(
            'Main', 'sd_jmx_enable')
    return sd_config


class StubStore(AbstractConfigStore):
    """Used when no valid config store was found. Allow to use auto_config."""
    def _extract_settings(self, config):
        pass

    def get_client(self):
        pass

    def crawl_config_template(self):
        # There is no user provided templates in auto_config mode
        return False
