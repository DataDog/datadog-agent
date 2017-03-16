from checks import AgentCheck


class TestCheck(AgentCheck):
    def check(self, instance):
        """
        Do not interact with the Aggregator during
        unit tests. Doing anything is ok here.
        """
        pass


class FooCheck:
    def __init__(self, foo, bar, baz):
        pass
