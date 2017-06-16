# project
from checks import AgentCheck


class Bad(AgentCheck):
    def check(self, instance):
        raise Exception('bad check!')
