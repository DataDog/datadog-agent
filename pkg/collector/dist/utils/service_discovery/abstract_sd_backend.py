# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# std
import logging
import re

# project
from utils.singleton import Singleton

log = logging.getLogger(__name__)


class AbstractSDBackend(object):
    """Singleton for service discovery backends"""
    __metaclass__ = Singleton

    PLACEHOLDER_REGEX = re.compile(r'%%.+?%%')

    def __init__(self, agentConfig=None):
        self.agentConfig = agentConfig
        # this variable is used to store the name of checks that need to
        # be reloaded at the end of the current collector run.
        # If a full config reload is required, it is set to True or a set.
        self.reload_check_configs = False

    @classmethod
    def _drop(cls):
        if cls in cls._instances:
            del cls._instances[cls]

    def get_configs(self):
        """Get the config for all docker containers running on the host."""
        raise NotImplementedError()

    def _render_template(self, init_config_tpl, instance_tpl, variables):
        """Replace placeholders in a template with the proper values.
           Return a tuple made of `init_config` and `instances`."""
        config = (init_config_tpl, instance_tpl)
        for tpl in config:
            for key in tpl:
                # iterate over template variables found in the templates
                for var in self.PLACEHOLDER_REGEX.findall(str(tpl[key])):
                    var_value = variables.get(var.strip('%'))
                    if var_value is not None:
                        # if the variable is found in a list (for example {'tags': ['%%tags%%', 'env:prod']})
                        # we need to iterate over its elements
                        if isinstance(tpl[key], list):
                            # if the variable is also a list we can just combine both lists
                            if isinstance(var_value, list):
                                tpl[key].remove(var)
                                tpl[key] += var_value
                                # remove dups
                                tpl[key] = list(set(tpl[key]))
                            else:
                                for idx, val in enumerate(tpl[key]):
                                    tpl[key][idx] = val.replace(var, var_value)
                        else:
                            if isinstance(var_value, list):
                                tpl[key] = var_value
                            else:
                                tpl[key] = tpl[key].replace(var, var_value)
                    else:
                        log.warning('Failed to interpolate variable {0} for the {1} parameter.'
                                    ' Dropping this configuration.'.format(var, key))
                        return None
        return config
