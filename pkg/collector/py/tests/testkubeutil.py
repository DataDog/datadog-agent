# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

from checks import AgentCheck
from kubeutil import get_connection_info


class TestCheck(AgentCheck):
    def check(self, instance):
        creds = get_connection_info()
        if creds.get("url"):
            self.warning("Found kubelet at " + creds.get("url"))
        else:
            self.warning("Kubelet not found")
        if creds.get("verify_tls") == "false":
            self.warning("no tls verification")
        if creds.get("ca_cert"):
            self.warning("ca_cert:" + creds.get("ca_cert"))
        if creds.get("client_crt"):
            self.warning("client_crt:" + creds.get("client_crt"))
        if creds.get("client_key"):
            self.warning("client_key:" + creds.get("client_key"))
        if creds.get("token"):
            self.warning("token:" + creds.get("token"))
