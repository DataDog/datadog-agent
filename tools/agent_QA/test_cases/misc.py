from test_builder import TestCase


class Serverless(TestCase):
    name = "[Serverless] Serverless log collection"

    def build(self, config):  # noqa: U100
        self.append(
            """
# Setup

Check if the latest Datadog Lambda Extension released has shipped some change from the release currently QAed. If so, validate that it is still capable of emitting logs, that they are visible in Datadog and correctly tagged.

# Test

- The Serverless Agent collects logs produced by an AWS lambda function
"""
        )


class StreamLogs(TestCase):
    name = "[Troubleshooting] Check that `agent stream-logs` works"

    def build(self, config):  # noqa: U100
        self.append(
            """
# Setup

- start the agent
- generate some logs `docker run -it bfloerschddog/flog -l > hello-world.log` and make sure the agent tails them

# Test

Ensure that the `agent stream-logs` command streams the logs, tags, and metadata

"""
        )
