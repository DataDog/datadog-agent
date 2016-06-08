from checks import AgentCheck

class TestCheck(AgentCheck):
    def check(self, instance):
        """
        Do not interact with the Aggregator during
        unit tests. Doing anything is ok here.
        """
        pass
