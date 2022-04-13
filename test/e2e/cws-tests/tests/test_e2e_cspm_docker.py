import os
import time
import unittest
import uuid
import warnings

from lib.config import gen_datadog_agent_config
from lib.const import CSPM_RUNNING_DOCKER_CHECK_LOG, CSPM_START_LOG
from lib.cspm.api import wait_for_compliance_event, wait_for_finding
from lib.cspm.finding import is_expected_docker_finding, parse_output_and_extract_findings
from lib.docker import DockerHelper
from lib.log import wait_agent_log
from lib.stepper import Step


class TestE2EDocker(unittest.TestCase):
    def setUp(self):
        warnings.simplefilter("ignore", category=ResourceWarning)
        warnings.simplefilter("ignore", category=UserWarning)

        self.docker_helper = DockerHelper()

    def tearDown(self):
        self.docker_helper.close()

    def test_privileged_container(self):
        print("")

        test_id = str(uuid.uuid4())[:4]
        agent_name = "security-agent"

        with Step(msg="create privileged container", emoji=":construction:"):
            pc = self.docker_helper.client.containers.run(
                "ubuntu:latest",
                command="sleep 7200",
                detach=True,
                remove=True,
                privileged=True,
            )
            self.container_id = pc.id

        with Step(msg="check agent start", emoji=":man_running:"):
            image = os.getenv("DD_AGENT_IMAGE")
            hostname = f"host_{test_id}"
            self.datadog_agent_config = gen_datadog_agent_config(
                hostname=hostname, log_level="DEBUG", tags=["tag1", "tag2"]
            )

            self.container = self.docker_helper.start_cspm_agent(
                image,
                datadog_agent_config=self.datadog_agent_config,
            )
            self.assertIsNotNone(self.container, msg="unable to start container")

            self.docker_helper.wait_agent_container()

            wait_agent_log(agent_name, self.docker_helper, CSPM_START_LOG)

        with Step(msg="check agent event", emoji=":check_mark_button:"):
            _, output = self.container.exec_run("security-agent compliance check --report")
            findings = parse_output_and_extract_findings(output.decode(), [CSPM_RUNNING_DOCKER_CHECK_LOG])
            self.finding = None
            for f in findings:
                if is_expected_docker_finding(f, self.container_id):
                    self.finding = f
            if self.finding is None:
                raise LookupError(f"{agent_name} | {CSPM_RUNNING_DOCKER_CHECK_LOG}")

        with Step(msg="wait for intake (~1m)", emoji=":alarm_clock:"):
            time.sleep(1 * 60)

        with Step(msg="check app compliance event", emoji=":SOON_arrow:"):
            wait_for_compliance_event(f"resource_id:*{self.container_id}")

        with Step(msg="wait for finding generation (~1m)", emoji=":alarm_clock:"):
            time.sleep(1 * 60)

        with Step(msg="check app finding", emoji=":chart_increasing_with_yen:"):
            wait_for_finding(f"@resource_type:docker_container @container_id:{self.container_id}")


def main():
    unittest.main()


if __name__ == "__main__":
    main()
