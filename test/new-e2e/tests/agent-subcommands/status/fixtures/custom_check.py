from checks import AgentCheck


class HelloCheck(AgentCheck):
    def check(self, instance):
        self.set_metadata('custom_metadata_key', 'custom_metadata_value')
