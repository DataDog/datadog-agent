# the following try/except block will make the custom check compatible with any Agent version
try:
    # first, try to import the base class from new versions of the Agent...
    from datadog_checks.base import AgentCheck
except ImportError:
    # ...if the above failed, the check is running in Agent version < 6.6.0
    from checks import AgentCheck

# content of the special variable __version__ will be shown in the Agent status page
__version__ = "1.0.0"


# flake8: noqa
class HelloCheck(AgentCheck):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.value = 123

    def check(self, instance):
        self.gauge('hello.world', self.value, tags=['TAG_KEY:TAG_VALUE'] + self.instance.get('tags', []))
        self.value += 10
