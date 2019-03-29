# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

from checks import AgentCheck
import tagger


class TestCheck(AgentCheck):
    def check(self, instance):
        lowtags = tagger.get_tags("test_entity", False)
        self.gauge("old_method.low_card", 1, tags=lowtags)

        alltags = tagger.get_tags("test_entity", True)
        self.gauge("old_method.high_card", 1, tags=alltags)

        notags = tagger.get_tags("404", True)
        self.gauge("old_method.unknown", 1, tags=notags)

        lowtags = tagger.tag("test_entity", tagger.LOW)
        self.gauge("new_method.low_card", 1, tags=lowtags)

        orchtags = tagger.tag("test_entity", tagger.ORCHESTRATOR)
        self.gauge("new_method.orch_card", 1, tags=orchtags)

        alltags = tagger.tag("test_entity", tagger.HIGH)
        self.gauge("new_method.high_card", 1, tags=alltags)

        notags = tagger.tag("404", tagger.LOW)
        self.gauge("new_method.unknown", 1, tags=notags)
