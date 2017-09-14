# (C) Datadog, Inc. 2017
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

from utils.kubernetes import KubeUtil
from utils.platform import Platform
from .baseutil import BaseUtil


class KubeUtilProxy(BaseUtil):
    def get_container_tags(self, cid=None, co=None):
        return None  # Kube tags are fetched directly

    @staticmethod
    def is_detected():
        return Platform.is_k8s()

    def get_host_tags(self):
        return KubeUtil().get_node_hosttags()
