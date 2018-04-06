# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

from checks import AgentCheck
from containers import is_excluded


containers = [
    # name, image, should_be_excluded
    ["dd-152462", "dummy:latest", True],
    ["dd-152462", "apache:latest", False],
    ["dummy", "dummy", False],
    ["dummy", "k8s.gcr.io/pause-amd64:3.1", True]
]


class TestCheck(AgentCheck):
    def check(self, instance):
        for c in containers:
            excluded = is_excluded(c[0], c[1])
            if excluded != c[2]:
                self.warning("Error, got {} for {}".format(excluded, c))
