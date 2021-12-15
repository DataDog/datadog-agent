import argparse
import sys
import time
import unittest
import warnings

import emoji
from lib.const import CSPM_RUNNING_K8S_MASTER_CHECK_LOG, CSPM_RUNNING_K8S_WORKER_CHECK_LOG, CSPM_START_LOG
from lib.cspm.api import wait_for_compliance_event, wait_for_finding
from lib.cspm.finding import (
    is_expected_k8s_master_node_finding,
    is_expected_k8s_worker_node_finding,
    parse_output_and_extract_findings,
)
from lib.kubernetes import KubernetesHelper
from lib.log import wait_agent_log
from lib.stepper import Step


class TestE2EKubernetes(unittest.TestCase):

    namespace = "default"
    in_cluster = False

    def setUp(self):
        warnings.simplefilter("ignore", category=ResourceWarning)
        warnings.simplefilter("ignore", category=UserWarning)
        warnings.simplefilter("ignore", category=DeprecationWarning)

        self.kubernetes_helper = KubernetesHelper(namespace=self.namespace, in_cluster=self.in_cluster)
        self.resource_id = "k8s-e2e-tests-control-plane_kubernetes_*_node"

    def test_k8s(self):
        print("")

        agent_name = "security-agent"

        with Step(msg="select pod", emoji=":man_running:"):
            self.kubernetes_helper.select_pod_name("app=datadog-agent")

        with Step(msg="check agent start", emoji=":man_running:"):
            wait_agent_log(agent_name, self.kubernetes_helper, CSPM_START_LOG)

        with Step(msg="check agent event", emoji=":check_mark_button:"):
            output = self.kubernetes_helper.exec_command(
                agent_name, ["security-agent", "compliance", "check", "--report"]
            )
            findings = parse_output_and_extract_findings(
                output,
                [CSPM_RUNNING_K8S_MASTER_CHECK_LOG, CSPM_RUNNING_K8S_WORKER_CHECK_LOG],
            )
            self.finding = None
            for f in findings:
                if is_expected_k8s_master_node_finding(f) or is_expected_k8s_worker_node_finding(f):
                    self.finding = f
            if self.finding is None:
                raise LookupError(
                    f"{agent_name} | {CSPM_RUNNING_K8S_MASTER_CHECK_LOG} | {CSPM_RUNNING_K8S_WORKER_CHECK_LOG}"
                )

        with Step(msg="wait for intake (~1m)", emoji=":alarm_clock:"):
            time.sleep(1 * 60)

        with Step(msg="check app compliance event", emoji=":SOON_arrow:"):
            wait_for_compliance_event(f"resource_id:{self.resource_id}")

        with Step(msg="wait for finding generation (~1m)", emoji=":alarm_clock:"):
            time.sleep(1 * 60)

        with Step(msg="check app finding", emoji=":chart_increasing_with_yen:"):
            wait_for_finding(f"@resource_type:kubernetes_*_node @resource:{self.resource_id}")

        print(emoji.emojize(":heart_on_fire:"), flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--namespace", default="default")
    parser.add_argument("--in-cluster", action="store_true")
    parser.add_argument("unittest_args", nargs="*")
    args = parser.parse_args()

    # setup some specific tests
    TestE2EKubernetes.namespace = args.namespace
    TestE2EKubernetes.in_cluster = args.in_cluster

    unit_argv = [sys.argv[0]] + args.unittest_args
    unittest.main(argv=unit_argv)


if __name__ == "__main__":
    main()
