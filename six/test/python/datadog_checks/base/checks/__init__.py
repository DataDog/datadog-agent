# AgentCheck stubs for testing
class AgentCheck(object):
    def __init__(self, *args, **kwargs):
        pass

    def run(self):
        return ""  # empty string means success

    @staticmethod
    def load_config(yaml_str):
        if yaml_str == "":
            return None
        else:
            return {}
