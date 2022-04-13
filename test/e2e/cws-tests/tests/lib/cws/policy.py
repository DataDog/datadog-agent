import yaml


class PolicyLoader:
    def load(self, policy_content):
        self.data = yaml.safe_load(policy_content)
        return self.data

    def get_rule_by_desc(self, desc):
        for rule in self.data["rules"]:
            if rule["description"] == desc:
                return rule
