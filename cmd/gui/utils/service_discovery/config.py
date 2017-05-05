# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# std
import logging

# project
from utils.service_discovery.sd_backend import SD_BACKENDS
from utils.service_discovery.config_stores import extract_sd_config, SD_CONFIG_BACKENDS

log = logging.getLogger(__name__)

def extract_agent_config(config):
    # get merged into the real agentConfig
    agentConfig = {}

    backend = config.get('Main', 'service_discovery_backend')
    agentConfig['service_discovery'] = True

    conf_backend = None
    if config.has_option('Main', 'sd_config_backend'):
        conf_backend = config.get('Main', 'sd_config_backend')

    if backend not in SD_BACKENDS:
        log.error("The backend {0} is not supported. "
                  "Service discovery won't be enabled.".format(backend))
        agentConfig['service_discovery'] = False

    if conf_backend is None:
        log.warning('No configuration backend provided for service discovery. '
                    'Only auto config templates will be used.')
    elif conf_backend not in SD_CONFIG_BACKENDS:
        log.error("The config backend {0} is not supported. "
                  "Only auto config templates will be used.".format(conf_backend))
        conf_backend = None
    agentConfig['sd_config_backend'] = conf_backend

    additional_config = extract_sd_config(config)
    agentConfig.update(additional_config)
    return agentConfig
