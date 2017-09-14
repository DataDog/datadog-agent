# (C) Datadog, Inc. 2017
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)


from . import NomadUtil, MesosUtil, ECSUtil
from .dockerutilproxy import DockerUtilProxy
from .kubeutilproxy import KubeUtilProxy

from utils.singleton import Singleton


class MetadataCollector():
    """
    Wraps several BaseUtil classes with autodetection and allows to query
    them through the same interface as BaseUtil classes

    See BaseUtil for apidoc
    """
    __metaclass__ = Singleton

    def __init__(self):
        self._utils = []  # [BaseUtil object]
        self._has_detected = False
        self.reset()

    def get_container_tags(self, cid=None, co=None):
        concat_tags = []
        for util in self._utils:
            tags = util.get_container_tags(cid, co)
            if tags:
                concat_tags.extend(tags)

        return concat_tags

    def invalidate_cache(self, events):
        for util in self._utils:
            util.invalidate_cache(events)

    def reset_cache(self):
        for util in self._utils:
            util.reset_cache()

    def get_host_tags(self):
        concat_tags = []
        for util in self._utils:
            meta = util.get_host_tags()
            if meta:
                concat_tags.extend(meta)

        return concat_tags

    def reset(self):
        """
        Trigger a new autodetection and reset underlying util classes
        """
        self._utils = []

        if DockerUtilProxy.is_detected():
            util = DockerUtilProxy()
            util.reset_cache()
            self._utils.append(util)
        else:
            # Skip orchestrator detection if docker not found: agent5 is docker-only anyway
            self._has_detected = False
            return

        if KubeUtilProxy.is_detected():
            util = KubeUtilProxy()
            util.reset_cache()
            self._utils.append(util)
        if MesosUtil.is_detected():
            util = MesosUtil()
            util.reset_cache()
            self._utils.append(util)
        if NomadUtil.is_detected():
            util = NomadUtil()
            util.reset_cache()
            self._utils.append(util)
        if ECSUtil.is_detected():
            util = ECSUtil()
            util.reset_cache()
            self._utils.append(util)

        self._has_detected = bool(self._utils)

    def has_detected(self):
        """
        Returns whether the tagger has detected orchestrators it handles
        If false, calling get_container_tags will return an empty list
        """
        return self._has_detected
