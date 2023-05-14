import os
import socket
import tempfile
import time
import unittest
import uuid
import warnings

from lib.config import gen_datadog_agent_config, gen_system_probe_config
from lib.const import SECURITY_START_LOG, SYS_PROBE_START_LOG
from lib.cws.app import App
from lib.cws.policy import PolicyLoader
from lib.docker import DockerHelper
from lib.log import wait_agent_log
from lib.stepper import Step


class TestE2EDocker(unittest.TestCase):
    def setUp(self):
        warnings.simplefilter("ignore", category=ResourceWarning)
        warnings.simplefilter("ignore", category=UserWarning)

        self.signal_rule_id = None
        self.agent_rule_id = None
        self.policies = None

        self.app = App()
        self.docker_helper = DockerHelper()
        self.policy_loader = PolicyLoader()

    def tearDown(self):
        if self.agent_rule_id:
            self.app.delete_agent_rule(self.agent_rule_id)

        if self.signal_rule_id:
            self.app.delete_signal_rule(self.signal_rule_id)

            self.docker_helper.close()

    def test_open_signal(self):
        print("")

        dir = tempfile.TemporaryDirectory(prefix="e2e-temp-folder")
        dirname = dir.name
        filename = f"{dirname}/secret"

        test_id = str(uuid.uuid4())[:4]
        desc = f"e2e test rule {test_id}"
        data = None
        agent_rule_name = f"e2e_agent_rule_{test_id}"
        rc_enabled = os.getenv("DD_RC_ENABLED") != ""

        with Step(msg=f"check agent rule({test_id}) creation", emoji=":straight_ruler:"):
            self.agent_rule_id = self.app.create_cws_agent_rule(
                agent_rule_name,
                desc,
                f'open.file.path == "{filename}"',
            )

        with Step(msg=f"check signal rule({test_id}) creation", emoji=":straight_ruler:"):
            self.signal_rule_id = self.app.create_cws_signal_rule(
                desc,
                "signal rule for e2e testing",
                agent_rule_name,
            )

        with Step(msg="check agent start", emoji=":man_running:"):
            image = os.getenv("DD_AGENT_IMAGE")
            hostname = f"host-{test_id}"
            self.datadog_agent_config = gen_datadog_agent_config(
                hostname=hostname,
                log_level="DEBUG",
                tags=["tag1", "tag2"],
                rc_enabled=rc_enabled,
            )
            self.system_probe_config = gen_system_probe_config(
                log_level="TRACE", log_patterns=["module.APIServer.*"], rc_enabled=rc_enabled
            )

            self.container = self.docker_helper.start_cws_agent(
                image,
                datadog_agent_config=self.datadog_agent_config,
                system_probe_config=self.system_probe_config,
            )
            self.assertIsNotNone(self.container, msg="unable to start container")

            self.docker_helper.wait_agent_container()

            wait_agent_log("security-agent", self.docker_helper, SECURITY_START_LOG)
            wait_agent_log("system-probe", self.docker_helper, SYS_PROBE_START_LOG)

        if rc_enabled:
            with Step(msg="check `remote-config` ruleset_loaded", emoji=":delivery_truck:"):
                event = self.app.wait_app_log("rule_id:ruleset_loaded @policies.source:remote-config")
                attributes = event["data"][-1]["attributes"]["attributes"]
                prev_load_date = attributes["date"]
                self.app.check_for_ignored_policies(self, attributes)

        with Step(msg="download policies", emoji=":file_folder:"):
            self.policies = self.docker_helper.download_policies().output.decode()
            self.assertNotEqual(self.policies, "", msg="download policies failed")
            data = self.policy_loader.load(self.policies)
            self.assertIsNotNone(data, msg="unable to load policy")

        with Step(msg="check rule presence in policies", emoji=":bullseye:"):
            rule = self.policy_loader.get_rule_by_desc(desc)
            self.assertIsNotNone(rule, msg="unable to find e2e rule")
            self.assertEqual(rule["id"], agent_rule_name)

        with Step(msg="push policies", emoji=":envelope:"):
            self.docker_helper.push_policies(self.policies)

        with Step(msg="reload policies", emoji=":file_folder:"):
            self.docker_helper.reload_policies()

        with Step(msg="check `downloaded` ruleset_loaded", emoji=":delivery_truck:"):
            for _i in range(60):  # retry 60 times
                event = self.app.wait_app_log("rule_id:ruleset_loaded  @policies.source:file")
                attributes = event["data"][-1]["attributes"]["attributes"]
                load_date = attributes["date"]
                # search for restart log until the timestamp differs
                if load_date != prev_load_date:
                    break
                time.sleep(1)
            else:
                self.fail("check ruleset_loaded timeouted")
            self.app.check_for_ignored_policies(self, attributes)

        with Step(msg="wait for host tags (3m)", emoji=":alarm_clock:"):
            time.sleep(3 * 60)

        with Step(msg="wait for datadog.security_agent.runtime.running metric", emoji="\N{beer mug}"):
            self.app.wait_for_metric("datadog.security_agent.runtime.running", host=socket.gethostname())

        with Step(msg="check agent event", emoji=":check_mark_button:"):
            os.system(f"touch {filename}")

            wait_agent_log(
                "system-probe",
                self.docker_helper,
                f"Sending event message for rule `{agent_rule_name}`",
            )

            wait_agent_log("security-agent", self.docker_helper, "Successfully posted payload to")

        with Step(msg="check app event", emoji=":chart_increasing_with_yen:"):
            event = self.app.wait_app_log(f"rule_id:{agent_rule_name}")
            attributes = event["data"][0]["attributes"]

            self.assertIn("tag1", attributes["tags"], "unable to find tag")
            self.assertIn("tag2", attributes["tags"], "unable to find tag")

        with Step(msg="check app signal", emoji=":1st_place_medal:"):
            tag = f"rule_id:{agent_rule_name}"
            signal = self.app.wait_app_signal(tag)
            attributes = signal["data"][0]["attributes"]

            self.assertIn(tag, attributes["tags"], "unable to find rule_id tag")
            self.assertEqual(
                agent_rule_name,
                attributes["attributes"]["agent"]["rule_id"],
                "unable to find rule_id tag attribute",
            )

        with Step(msg="wait for datadog.security_agent.runtime.containers_running metric", emoji="\N{beer mug}"):
            self.app.wait_for_metric("datadog.security_agent.runtime.containers_running", host=socket.gethostname())


def main():
    unittest.main()


if __name__ == "__main__":
    main()
