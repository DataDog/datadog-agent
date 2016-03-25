import json
import traceback

AGENT_METRICS_CHECK_NAME = 'agent_metrics'

class AgentCheck():

    RATE = "rate"
    GAUGE = "gauge"

    def __init__(self):
        self.metrics = []
        self.results = []

    def gauge(self, name, value, tags=None):
        self.metrics.append((AgentCheck.GAUGE, name, value, tags))

    def rate(self, name, value, tags=None):
        self.metrics.append((AgentCheck.RATE, name, value, tags))

    def check(self, instance):
        raise NotImplementedError

    def run_instance(self, instance):
        self.check(instance)

    def run(self, *args):
        try:
            instances, init_config = args

            for i in instances:
                self.run_instance(i)

            return json.dumps(self.metrics)

        except Exception, e:
            return json.dumps(
                [
                    {
                        "message": str(e),
                        "traceback": traceback.format_exc(),
                    }
                ]
            )
