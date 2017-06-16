from checks import AgentCheck


class TestCheck(AgentCheck):
    def __init__(self, *args, **kwargs):
        super(TestCheck, self).__init__(*args, **kwargs)
        assert hasattr(self, 'instances')
        assert isinstance(self.instances, list)
        assert len(self.instances) > 0
        assert 'foo' in self.instances[0]

    def check(self, instance):
        pass
