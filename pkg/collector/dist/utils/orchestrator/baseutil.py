# (C) Datadog, Inc. 2017
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# stdlib
import logging
import requests

# project
from utils.dockerutil import DockerUtil
from utils.singleton import Singleton


class BaseUtil:
    """
    Base class for orchestrator utils. Only handles container tags for now.
    Users should go through the orchestrator.Tagger class to simplify the code

    Children classes can implement:
      - __init__: to change self.needs_inspect
      - _get_cacheable_tags: tags will be cached for reuse
      - _get_transient_tags: tags can change and won't be cached (TODO)
      - invalidate_cache: custom cache invalidation logic
      - is_detected (staticmethod)
    """
    __metaclass__ = Singleton

    def __init__(self):
        # Whether your get___tags methods need the Config section inspect data
        self.needs_inspect_config = False
        # Whether your get___tags methods need the Labels section inspect data
        self.needs_inspect_labels = False

        self.log = logging.getLogger(__name__)
        self.docker_util = DockerUtil()

        # Tags cache as a dict {co_id: [tags]}
        self._container_tags_cache = {}

    def get_container_tags(self, cid=None, co=None):
        """
        Returns container tags for the given container, inspecting the container if needed
        :param container: either the container id or container dict returned by docker-py
        :return: tags as list<string>, cached
        """

        if (cid is not None) and (co is not None):
            self.log.error("Can only pass either a container id or object, not both, returning empty tags")
            return []
        if (cid is None) and (co is None):
            self.log.error("Need one container id or container object, returning empty tags")
            return []
        elif co is not None:
            if 'Id' in co:
                cid = co.get('Id')
            else:
                self.log.warning("Invalid container dict, returning empty tags")
                return []

        if cid in self._container_tags_cache:
            return self._container_tags_cache[cid]
        else:
            if self.needs_inspect_config and (co is None or 'Config' not in co):
                co = self.docker_util.inspect_container(cid)
            if self.needs_inspect_labels and (co is None or 'Labels' not in co):
                co = self.docker_util.inspect_container(cid)

            self._container_tags_cache[cid] = self._get_cacheable_tags(cid, co)
            return self._container_tags_cache[cid]

    def invalidate_cache(self, events):
        """
        Allows cache invalidation when containers die
        :param events from self.get_events
        """
        try:
            for ev in events:
                if ev.get('status') == 'die' and ev.get('id') in self._container_tags_cache:
                    del self._container_tags_cache[ev.get('id')]
        except Exception as e:
            self.log.warning("Error when invalidating tag cache: " + str(e))

    def reset_cache(self):
        """
        Empties all caches to reset the singleton to initial state
        """
        self._container_tags_cache = {}

    # Util methods for children classes

    def _try_urls(self, urls, validation_lambda=None, timeout=1):
        """
        When detecting orchestrator agents, one might need to try several IPs
        before finding the good one.
        The first url returning a 200 and validating the lambda will be returned.
        If no lambda is provided, the first url to return a 200 is returned.
        :param urls: list of urls to try
        :param validation_lambda: lambda to return a boolean from a Request.Response
        :return: first url matching, or None
        """
        if not urls:
            return None

        for url in urls:
            try:
                response = requests.get(url, timeout=timeout)
                if response.status_code is not requests.codes.ok:
                    continue
                if validation_lambda and not validation_lambda(response):
                    continue
                return url
            except requests.exceptions.RequestException:  # Network
                continue
            except ValueError:  # JSON parsing or dict search
                continue
            except TypeError:  # NoneType errors
                continue

        return None
