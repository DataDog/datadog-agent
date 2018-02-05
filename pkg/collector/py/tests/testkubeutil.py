# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

from checks import AgentCheck
from kubeutil import get_connection_info


class TestCheck(AgentCheck):
    def check(self, instance):
        creds = get_connection_info()
        if creds.get("url"):
            self.warning("Found kubelet at " + creds.get("url"))
        else:
            self.warning("Kubelet not found")
