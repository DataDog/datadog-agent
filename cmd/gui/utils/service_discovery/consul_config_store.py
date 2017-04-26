# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# project
from consul import Consul
from utils.service_discovery.abstract_config_store import AbstractConfigStore, KeyNotFound


DEFAULT_CONSUL_HOST = '127.0.0.1'
DEFAULT_CONSUL_PORT = 8500
DEFAULT_CONSUL_TOKEN = None
DEFAULT_CONSUL_SCHEME = 'http'
DEFAULT_CONSUL_CONSISTENCY = 'default'
DEFAULT_CONSUL_DATACENTER = None
DEFAULT_CONSUL_VERIFY = True


class ConsulStore(AbstractConfigStore):
    """Implementation of a config store client for consul"""
    def _extract_settings(self, config):
        """Extract settings from a config object"""
        settings = {
            'host': config.get('sd_backend_host', DEFAULT_CONSUL_HOST),
            'port': int(config.get('sd_backend_port', DEFAULT_CONSUL_PORT)),
            # all these are set to their default value for now
            'token': config.get('consul_token', None),
            'scheme': config.get('consul_scheme', DEFAULT_CONSUL_SCHEME),
            'consistency': config.get('consul_consistency', DEFAULT_CONSUL_CONSISTENCY),
            'verify': config.get('consul_verify', DEFAULT_CONSUL_VERIFY),
        }
        return settings

    def get_client(self, reset=False):
        """Return a consul client, create it if needed"""
        if self.client is None or reset is True:
            self.client = Consul(
                host=self.settings.get('host'),
                port=self.settings.get('port'),
                token=self.settings.get('token'),
                scheme=self.settings.get('scheme'),
                consistency=self.settings.get('consistency'),
                verify=self.settings.get('verify'),
            )
        return self.client

    def client_read(self, path, **kwargs):
        """Retrieve a value from a consul key."""
        recurse = kwargs.get('recursive') or kwargs.get('all', False)
        res = self.client.kv.get(path, recurse=recurse)
        if kwargs.get('watch', False):
            return res[0]
        else:
            if res[1] is not None:
                return res[1].get('Value') if not recurse else res[1]
            else:
                raise KeyNotFound("The key %s was not found in consul" % path)

    def dump_directory(self, path, **kwargs):
        """Return a dict made of all image names and their corresponding check info"""
        templates = {}
        path = path.lstrip('/')
        try:
            directory = self.client_read(
                path,
                recursive=True,
            )
        except KeyNotFound:
            raise KeyNotFound("The key %s was not found in consul" % path)
        for leaf in directory:
            image = leaf.get('Key').split('/')[-2]
            param = leaf.get('Key').split('/')[-1]
            value = leaf.get('Value')
            if image not in templates:
                templates[image] = {}
            templates[image][param] = value

        return templates
