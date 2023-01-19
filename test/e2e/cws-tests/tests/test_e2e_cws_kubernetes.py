import argparse
import os
import sys
import tempfile
import time
import unittest
import uuid
import warnings

import emoji
from lib.const import SECURITY_START_LOG, SYS_PROBE_START_LOG
from lib.cws.app import App
from lib.cws.policy import PolicyLoader
from lib.kubernetes import KubernetesHelper
from lib.log import wait_agent_log
from lib.stepper import Step


class TestE2EKubernetes(unittest.TestCase):
    namespace = "default"
    in_cluster = False
    hostname = "k8s-e2e-tests-control-plane"

    def setUp(self):
        warnings.simplefilter("ignore", category=ResourceWarning)
        warnings.simplefilter("ignore", category=UserWarning)
        warnings.simplefilter("ignore", category=DeprecationWarning)

        self.signal_rule_id = None
        self.agent_rule_id = None
        self.policies = None

        self.app = App()
        self.kubernetes_helper = KubernetesHelper(namespace=self.namespace, in_cluster=self.in_cluster)
        self.policy_loader = PolicyLoader()

    def tearDown(self):
        if self.agent_rule_id:
            self.app.delete_agent_rule(self.agent_rule_id)

        if self.signal_rule_id:
            self.app.delete_signal_rule(self.signal_rule_id)

    def test_open_signal(self):
        print("")

        dir = tempfile.TemporaryDirectory(prefix="e2e-temp-folder")
        dirname = dir.name
        filename = f"{dirname}/secret"

        test_id = str(uuid.uuid4())[:4]
        desc = f"e2e test rule {test_id}"
        data = None
        agent_rule_name = f"e2e_agent_rule_{test_id}"

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

        with Step(msg="select pod", emoji=":man_running:"):
            self.kubernetes_helper.select_pod_name("app=datadog-agent")

        with Step(msg="check security-agent start", emoji=":customs:"):
            wait_agent_log("security-agent", self.kubernetes_helper, SECURITY_START_LOG)

        with Step(msg="check system-probe start", emoji=":customs:"):
            wait_agent_log("system-probe", self.kubernetes_helper, SYS_PROBE_START_LOG)

        with Step(msg="copy default policies", emoji=":delivery_truck:"):
            self.kubernetes_helper.exec_command("security-agent", command=["mkdir", "-p", "/tmp/runtime-security.d"])
            command = [
                "cp",
                "/etc/datadog-agent/runtime-security.d/default.policy",
                "/tmp/runtime-security.d/default.policy",
            ]
            self.kubernetes_helper.exec_command("security-agent", command=command)

        with Step(msg="reload policies", emoji=":rocket:"):
            self.kubernetes_helper.reload_policies()

        with Step(msg="check `file` ruleset_loaded", emoji=":delivery_truck:"):
            event = self.app.wait_app_log("rule_id:ruleset_loaded @policies.source:file")
            attributes = event["data"][-1]["attributes"]["attributes"]
            prev_load_date = attributes["date"]
            self.app.check_for_ignored_policies(self, attributes)

        with Step(msg="check `remote-config` ruleset_loaded", emoji=":delivery_truck:"):
            event = self.app.wait_app_log("rule_id:ruleset_loaded @policies.source:remote-config")
            attributes = event["data"][-1]["attributes"]["attributes"]
            prev_load_date = attributes["date"]
            self.app.check_for_ignored_policies(self, attributes)

        with Step(msg="wait for host tags (3m)", emoji=":alarm_clock:"):
            time.sleep(3 * 60)

        with Step(msg="check policies download", emoji=":file_folder:"):
            self.policies = self.kubernetes_helper.download_policies()
            self.assertNotEqual(self.policies, "", msg="check policy download failed")
            data = self.policy_loader.load(self.policies)
            self.assertIsNotNone(data, msg="unable to load policy")

        with Step(msg="check rule presence in policies", emoji=":bullseye:"):
            rule = self.policy_loader.get_rule_by_desc(desc)
            self.assertIsNotNone(rule, msg="unable to find e2e rule")
            self.assertEqual(rule["id"], agent_rule_name)

        with Step(msg="push policies", emoji=":envelope:"):
            self.kubernetes_helper.push_policies(self.policies)

        with Step(msg="reload policies", emoji=":rocket:"):
            self.kubernetes_helper.reload_policies()

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

        with Step(msg="wait for datadog.security_agent.runtime.running metric", emoji="\N{beer mug}"):
            self.app.wait_for_metric("datadog.security_agent.runtime.running", host=TestE2EKubernetes.hostname)

        with Step(msg="check agent event", emoji=":check_mark_button:"):
            os.system(f"touch {filename}")

            wait_agent_log(
                "system-probe",
                self.kubernetes_helper,
                f"Sending event message for rule `{agent_rule_name}`",
            )

            wait_agent_log(
                "security-agent",
                self.kubernetes_helper,
                "Successfully posted payload to",
            )

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
            self.app.wait_for_metric(
                "datadog.security_agent.runtime.containers_running", host=TestE2EKubernetes.hostname
            )

        print(emoji.emojize(":heart_on_fire:"), flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--namespace", default="default")
    parser.add_argument("--in-cluster", action='store_true')
    parser.add_argument("unittest_args", nargs="*")
    args = parser.parse_args()

    # setup some specific tests
    TestE2EKubernetes.namespace = args.namespace
    TestE2EKubernetes.in_cluster = args.in_cluster

    unit_argv = [sys.argv[0]] + args.unittest_args
    unittest.main(argv=unit_argv)


if __name__ == "__main__":
    main()
