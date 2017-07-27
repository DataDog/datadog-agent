from datetime import datetime
from checks import AgentCheck


class TestAggregatorCheck(AgentCheck):
    def check(self, instance):
        """
        Do not interact with the Aggregator during
        unit tests. Doing anything is ok here.
        """
        self.service_check("testservicecheck", AgentCheck.OK, tags=None, message="")
        # _send_metric is not used in tests, so it should not be used to test it.
        # Instead call gauge, which is the one that checks will be using
        self.gauge("testmetric", 0, tags=None)
        self.event({
            "event_type": "new.event",
            "msg_title": "new test event",
            "aggregation_key": "test.event",
            "msg_text": "test event test event",
            "tags": None
        })
