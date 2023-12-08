import random
import string
from datadog_checks.checks import AgentCheck


def generate_random_tags(min_tags, max_tags):
    def random_string(length=5):
        # Generate a random string of fixed length
        return ''.join(random.choice(string.ascii_lowercase) for _ in range(length))

    num_tags = random.randint(min_tags, max_tags)
    tags = []

    for _ in range(num_tags):
        key = random_string()
        value = random_string()
        tag = f"{key}:{value}"
        tags.append(tag)

    return tags

class MyCheck(AgentCheck):
    def check(self, instance):
        total_tags = 0

        for _ in range(10):
            tags = generate_random_tags(5, 50)
            total_tags += len(tags)
            self.gauge('my.metric', 1, tags=tags)

        self.gauge('py.num_submitted_tags', total_tags)
        print(f"Submitted {total_tags} tags across 10 metrics")

