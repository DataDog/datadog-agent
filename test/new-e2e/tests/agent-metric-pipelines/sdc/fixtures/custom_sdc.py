# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

from datadog_checks.base import AgentCheck


class CustomSdcCheck(AgentCheck):
    def check(self, _):
        counter = getattr(self, "counter", 0) + 1
        self.counter = counter
        tags = ["compression:sdc", "fixture:custom-check"]
        self.gauge("e2e.sdc.gauge", 42, tags=tags)
        self.rate("e2e.sdc.rate", counter, tags=tags)
