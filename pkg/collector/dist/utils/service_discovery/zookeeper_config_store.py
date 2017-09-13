# (C) Datadog, Inc. 2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# std
import logging

from kazoo.client import KazooClient, NoNodeError
from utils.service_discovery.abstract_config_store import AbstractConfigStore, KeyNotFound

DEFAULT_ZK_HOST = '127.0.0.1'
DEFAULT_ZK_PORT = 2181
DEFAULT_TIMEOUT = 5
log = logging.getLogger(__name__)

class ZookeeperStore(AbstractConfigStore):
    """Implementation of a config store client for Zookeeper"""

    def _extract_settings(self, config):
        """Extract settings from a config object"""
        settings = {
            'host': config.get('sd_backend_host', DEFAULT_ZK_HOST),
            'port': int(config.get('sd_backend_port', DEFAULT_ZK_PORT)),
        }
        return settings

    def get_client(self, reset=False):
        if self.client is None or reset is True:
            self.client = KazooClient(
                hosts=self.settings.get('host') + ":" + str(self.settings.get('port')),
                read_only=True,
            )
            self.client.start()
        return self.client

    def client_read(self, path, **kwargs):
        """Retrieve a value from a Zookeeper key."""
        try:
            if kwargs.get('watch', False):
                return self.recursive_mtime(path)
            elif kwargs.get('all', False):
                # we use it in _populate_identifier_to_checks
                results = []
                self.recursive_list(path, results)
                return results
            else:
                res, stats = self.client.get(path)
                return res.decode("utf-8")
        except NoNodeError:
            raise KeyNotFound("The key %s was not found in Zookeeper" % path)

    def recursive_list(self, path, results):
        """Recursively walks the children from the given path and build a list of key/value tuples"""
        try:
            data, stat = self.client.get(path)

            if data:
                node_as_string = data.decode("utf-8")
                if not node_as_string:
                    results.append((path.decode("utf-8"), node_as_string))

            children = self.client.get_children(path)
            if children is not None:
                for child in children:
                    new_path = '/'.join([path.rstrip('/'), child])
                    self.recursive_list(new_path, results)
        except NoNodeError:
            raise KeyNotFound("The key %s was not found in Zookeeper" % path)

    def recursive_mtime(self, path):
        """Recursively walks the children from the given path to find the maximum modification time"""
        try:
            data, stat = self.client.get(path)
            children = self.client.get_children(path)
            if children is not None and len(children) > 0:
                for child in children:
                    new_path = '/'.join([path.rstrip('/'), child])
                return max(stat.mtime, self.recursive_mtime(new_path))
            else:
                return stat.mtime
        except NoNodeError:
            raise KeyNotFound("The key %s was not found in Zookeeper" % path)

    def dump_directory(self, path, **kwargs):
        """Return a dict made of all image names and their corresponding check info"""
        templates = {}
        paths = []
        self.recursive_list(path, paths)

        for pair in paths:
            splits = pair[0].split('/')
            image = splits[-2]
            param = splits[-1]
            value = pair[1]
            if image not in templates:
                templates[image] = {}
            templates[image][param] = value

        return templates
