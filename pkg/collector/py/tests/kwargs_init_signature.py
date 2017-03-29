from agent import AgentCheck


class TestCheck(AgentCheck):
    def __init__(self, *args, **kwargs):
        super(TestCheck, self).__init__(*args, **kwargs)

    def check(self, instance):
        pass
