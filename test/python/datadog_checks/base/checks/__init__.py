import sys


# AgentCheck stubs for testing
class __AgentCheckPy3(object):
    def __init__(self, *args, **kwargs):
        pass

    def run(self):
        return "result"

    @staticmethod
    def load_config(yaml_str):
        pass


class __AgentCheckPy2(object):
    def __init__(self, *args, **kwargs):
        pass

    def run(self):
        return "result"

    @classmethod
    def load_config(cls, yaml_str):
        pass


if sys.version_info[0] == 3:
    AgentCheck = __AgentCheckPy3
    del __AgentCheckPy2
else:
    AgentCheck = __AgentCheckPy2
    del __AgentCheckPy3