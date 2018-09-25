# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

from datetime import datetime
from checks import AgentCheck


class TestAggregatorCheck(AgentCheck):
    def check(self, instance):
        """
        Do not interact with the Aggregator during
        unit tests. Doing anything is ok here.
        """
        self.service_check("testservicecheck", AgentCheck.OK, tags=None, message="")
        self.service_check("testservicecheckwithhostname", AgentCheck.OK, tags=["foo", "bar"], hostname="testhostname", message="a message")
        self.service_check("testservicecheckwithnonemessage", AgentCheck.OK, message=None)

        # _send_metric is not used in tests, so it should not be used to test it.
        # Instead call gauge, which is the one that checks will be using
        self.gauge("testmetric", 0, tags=None)
        self.gauge("testmetricnonevalue", None) # metrics with None values should be ignored by AgentCheck
        self.gauge("testmetricstringvalue", "2") # string values should be cast to floats
        self.gauge("testmetricstringvalue", "notcastabletofloat") # values not castable to floats shouldn't raise an exception, but won't be submitted

        self.increment("test.increment", tags=['foo', 'bar'])
        self.decrement("test.decrement", tags=['foo', 'bar', 'baz'])

        self.event({
            "event_type": "new.event",
            "msg_title": "new test event",
            "aggregation_key": "test.event",
            "msg_text": "test event test event",
            "tags": None
        })
