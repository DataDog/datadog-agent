# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

from checks import AgentCheck
from tagger import get_tags


class TestCheck(AgentCheck):
    def check(self, instance):
        lowtags = get_tags("test_entity", False)
        self.gauge("metric.low_card", 1, tags=lowtags)

        alltags = get_tags("test_entity", True)
        self.gauge("metric.high_card", 1, tags=alltags)

        notags = get_tags("404", True)
        self.gauge("metric.unknown", 1, tags=notags)
