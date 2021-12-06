import os
import tempfile
import time
import unittest
import uuid
import warnings

from lib.app import App
from lib.config import gen_datadog_agent_config, gen_system_probe_config
from lib.const import SECURITY_START_LOG, SYS_PROBE_START_LOG
from lib.docker import DockerHelper
from lib.log import wait_agent_log
from lib.policy import PolicyLoader
from lib.stepper import Step


class TestE2EDocker(unittest.TestCase):
    def setUp(self):
        warnings.simplefilter("ignore", category=ResourceWarning)
        warnings.simplefilter("ignore", category=UserWarning)

        self.rule_id = None
        self.policies = None

        self.App = App()
        self.docker_helper = DockerHelper()
        self.policy_loader = PolicyLoader()

    def tearDown(self):
        if self.rule_id:
            self.App.delete_rule(self.rule_id)

        if self.policies:
            os.remove(self.policies)

            self.docker_helper.close()

    def test_open_signal(self):
        print("")

        dir = tempfile.TemporaryDirectory(prefix="e2e-temp-folder")
        dirname = dir.name
        filename = f"{dirname}/secret"

        test_id = str(uuid.uuid4())[:4]
        desc = f"e2e test rule {test_id}"
        data = None

        with Step(msg=f"check rule({test_id}) creation", emoji=":straight_ruler:"):
            self.rule_id = self.App.create_cws_rule(
                desc,
                "rule for e2e testing",
                f"e2e_{test_id}",
                f'open.file.path == "{filename}"',
            )

        with Step(msg="check policies download", emoji=":file_folder:"):
            self.policies = self.App.download_policies()
            data = self.policy_loader.load(self.policies)
            self.assertIsNotNone(data, msg="unable to load policy")

        with Step(msg="check rule presence in policies", emoji=":bullseye:"):
            rule = self.policy_loader.get_rule_by_desc(desc)
            self.assertIsNotNone(rule, msg="unable to find e2e rule")

        with Step(msg="check agent start", emoji=":man_running:"):
            image = os.getenv("DD_AGENT_IMAGE")
            hostname = f"host_{test_id}"
            self.datadog_agent_config = gen_datadog_agent_config(
                hostname=hostname, log_level="DEBUG", tags=["tag1", "tag2"]
            )
            self.system_probe_config = gen_system_probe_config(log_level="TRACE", log_patterns=["module.APIServer.*"])

            self.container = self.docker_helper.start_agent(
                image,
                self.policies,
                datadog_agent_config=self.datadog_agent_config,
                system_probe_config=self.system_probe_config,
            )
            self.assertIsNotNone(self.container, msg="unable to start container")

            self.docker_helper.wait_agent_container()

            wait_agent_log("security-agent", self.docker_helper, SECURITY_START_LOG)
            wait_agent_log("system-probe", self.docker_helper, SYS_PROBE_START_LOG)

        with Step(msg="wait for host tags(~2m)", emoji=":alarm_clock:"):
            time.sleep(3 * 60)

        with Step(msg="check agent event", emoji=":check_mark_button:"):
            os.system(f"touch {filename}")

            rule_id = rule["id"]

            wait_agent_log(
                "system-probe",
                self.docker_helper,
                f"Sending event message for rule `{rule_id}`",
            )

        with Step(msg="check app event", emoji=":chart_increasing_with_yen:"):
            rule_id = rule["id"]
            event = self.App.wait_app_log(f"rule_id:{rule_id}")
            attributes = event["data"][0]["attributes"]

            self.assertIn("tag1", attributes["tags"], "unable to find tag")
            self.assertIn("tag2", attributes["tags"], "unable to find tag")

        with Step(msg="check app signal", emoji=":1st_place_medal:"):
            rule_id = rule["id"]
            tag = f"rule_id:{rule_id}"
            signal = self.App.wait_app_signal(tag)
            attributes = signal["data"][0]["attributes"]

            self.assertIn(tag, attributes["tags"], "unable to find rule_id tag")
            self.assertEqual(
                rule["id"],
                attributes["attributes"]["agent"]["rule_id"],
                "unable to find rule_id tag attribute",
            )


def main():
    unittest.main()


if __name__ == "__main__":
    main()
