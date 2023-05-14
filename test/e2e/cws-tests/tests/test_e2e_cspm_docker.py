import json
import os
import socket
import time
import unittest
import uuid
import warnings

from lib.config import gen_datadog_agent_config
from lib.const import CSPM_START_LOG
from lib.cspm.api import App
from lib.docker import DockerHelper
from lib.log import wait_agent_log
from lib.stepper import Step
from test_e2e_cspm import expect_findings


class TestE2EDocker(unittest.TestCase):
    def setUp(self):
        warnings.simplefilter("ignore", category=ResourceWarning)
        warnings.simplefilter("ignore", category=UserWarning)

        self.docker_helper = DockerHelper()
        self.app = App()

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

        with Step(msg="check agent events", emoji=":check_mark_button:"):
            self.container.exec_run("security-agent compliance check --dump-reports /tmp/reports.json --report")
            _, output = self.container.exec_run("cat /tmp/reports.json")
            findings = json.loads(output)

            expected_findings = {
                "cis-docker-1.2.0-5.4": [
                    {
                        "agent_rule_id": "cis-docker-1.2.0-5.4",
                        "agent_framework_id": "cis-docker",
                        "result": "failed",
                        "resource_type": "docker_container",
                        "data": {
                            "container.id": self.container_id,
                        },
                    }
                ],
                "cis-docker-1.2.0-1.2.1": [{"result": "failed"}],
                "cis-docker-1.2.0-1.2.3": [{"result": "error"}],
                "cis-docker-1.2.0-1.2.4": [{"result": "error"}],
                "cis-docker-1.2.0-1.2.5": [{"result": "error"}],
                "cis-docker-1.2.0-1.2.6": [{"result": "error"}],
                "cis-docker-1.2.0-1.2.7": [{"result": "error"}],
                "cis-docker-1.2.0-1.2.8": [{"result": "error"}],
                "cis-docker-1.2.0-1.2.9": [{"result": "error"}],
                "cis-docker-1.2.0-1.2.10": [{"result": "error"}],
                "cis-docker-1.2.0-1.2.11": [{"result": "error"}],
                "cis-docker-1.2.0-1.2.12": [{"result": "error"}],
                "cis-docker-1.2.0-2.2": [{"result": "failed"}],
                "cis-docker-1.2.0-2.3": [{"result": "failed"}],
                "cis-docker-1.2.0-2.4": [{"result": "failed"}],
                "cis-docker-1.2.0-2.6": [{"result": "failed"}],
                "cis-docker-1.2.0-3.10": [{"result": "error"}],
                "cis-docker-1.2.0-3.11": [{"result": "error"}],
                "cis-docker-1.2.0-3.12": [{"result": "error"}],
                "cis-docker-1.2.0-3.13": [{"result": "error"}],
                "cis-docker-1.2.0-3.14": [{"result": "error"}],
                "cis-docker-1.2.0-3.15": [{"result": "error"}],
                "cis-docker-1.2.0-3.16": [{"result": "error"}],
                "cis-docker-1.2.0-3.17": [{"result": "error"}],
                "cis-docker-1.2.0-3.18": [{"result": "error"}],
                "cis-docker-1.2.0-3.19": [{"result": "error"}],
                "cis-docker-1.2.0-3.20": [{"result": "error"}],
                "cis-docker-1.2.0-3.21": [{"result": "error"}],
                "cis-docker-1.2.0-3.22": [{"result": "error"}],
                "cis-docker-1.2.0-3.7": [{"result": "error"}],
                "cis-docker-1.2.0-3.8": [{"result": "error"}],
                "cis-docker-1.2.0-3.9": [{"result": "error"}],
                "cis-docker-1.2.0-4.1": [{"result": "failed"}],
                "cis-docker-1.2.0-4.6": [{"result": "failed"}],
                "cis-docker-1.2.0-5.1": [{"result": "failed"}],
                "cis-docker-1.2.0-5.10": [{"result": "failed"}],
                "cis-docker-1.2.0-5.11": [{"result": "failed"}],
                "cis-docker-1.2.0-5.12": [{"result": "failed"}],
                "cis-docker-1.2.0-5.14": [{"result": "failed"}],
                "cis-docker-1.2.0-5.2": [{"result": "error"}],
                "cis-docker-1.2.0-5.25": [{"result": "failed"}],
                "cis-docker-1.2.0-5.26": [{"result": "failed"}],
                "cis-docker-1.2.0-5.28": [{"result": "failed"}],
                "cis-docker-1.2.0-5.31": [{"result": "failed"}],
                "cis-docker-1.2.0-5.7": [{"result": "failed"}],
            }

            expect_findings(self, findings, expected_findings)

        with Step(msg="wait for intake (~1m)", emoji=":alarm_clock:"):
            time.sleep(1 * 60)

        with Step(msg="wait for datadog.security_agent.compliance.running metric", emoji="\N{beer mug}"):
            self.app.wait_for_metric("datadog.security_agent.compliance.running", host=socket.gethostname())

        ## Disabled while no CSPM API is available
        # with Step(msg="check app compliance event", emoji=":SOON_arrow:"):
        #    wait_for_compliance_event(f"resource_id:*{self.container_id}")

        with Step(msg="wait for finding generation (~1m)", emoji=":alarm_clock:"):
            time.sleep(1 * 60)

        with Step(msg="wait for datadog.security_agent.compliance.containers_running metric", emoji="\N{beer mug}"):
            self.app.wait_for_metric(
                "datadog.security_agent.compliance.containers_running", container_id=self.container_id
            )

        ## Disabled while no CSPM API is available
        # with Step(msg="check app finding", emoji=":chart_increasing_with_yen:"):
        #    wait_for_findings(f"@resource_type:docker_container @container_id:{self.container_id}")


def main():
    unittest.main()


if __name__ == "__main__":
    main()
