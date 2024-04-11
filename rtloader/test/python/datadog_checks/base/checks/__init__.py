# AgentCheck stubs for testing
class AgentCheck(object):  # noqa
    def __init__(self, *args, **kwargs):  # noqa: U100
        pass

    def cancel(self):
        pass

    def run(self):
        return ""  # empty string means success

    @staticmethod
    def load_config(yaml_str):
        if yaml_str == "":
            return None
        else:
            return {}
