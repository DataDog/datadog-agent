import datetime
import os
import tempfile

import lib.common.app as common
import requests
from datadog_api_client.v2 import ApiClient, ApiException, Configuration
from datadog_api_client.v2.api import cloud_workload_security_api, logs_api, security_monitoring_api
from datadog_api_client.v2.models import (
    CloudWorkloadSecurityAgentRuleCreateAttributes,
    CloudWorkloadSecurityAgentRuleCreateData,
    CloudWorkloadSecurityAgentRuleCreateRequest,
    CloudWorkloadSecurityAgentRuleType,
    LogsListRequest,
    LogsListRequestPage,
    LogsQueryFilter,
    LogsSort,
    SecurityMonitoringRuleCaseCreate,
    SecurityMonitoringRuleCreatePayload,
    SecurityMonitoringRuleDetectionMethod,
    SecurityMonitoringRuleEvaluationWindow,
    SecurityMonitoringRuleKeepAlive,
    SecurityMonitoringRuleMaxSignalDuration,
    SecurityMonitoringRuleOptions,
    SecurityMonitoringRuleQueryAggregation,
    SecurityMonitoringRuleSeverity,
    SecurityMonitoringRuleTypeCreate,
    SecurityMonitoringSignalListRequest,
    SecurityMonitoringSignalListRequestFilter,
    SecurityMonitoringSignalListRequestPage,
    SecurityMonitoringSignalsSort,
    SecurityMonitoringStandardRuleQuery,
)
from retry.api import retry_call


def get_app_log(api_client, query):
    # ensures that we are filtering the logs from the e2e runs and not
    # other agents from people doing QA
    query = f"app:agent-e2e-tests host:k8s-e2e-tests-control-plane {query}"

    api_instance = logs_api.LogsApi(api_client)
    body = LogsListRequest(
        filter=LogsQueryFilter(
            _from="now-10m",
            indexes=["*"],
            query=query,
            to="now",
        ),
        page=LogsListRequestPage(
            limit=25,
        ),
        sort=LogsSort("timestamp"),
    )

    api_response = api_instance.list_logs(body=body)
    if not api_response["data"]:
        raise LookupError(query)

    return api_response


def get_app_signal(api_client, query):
    now = datetime.datetime.now(datetime.timezone.utc)
    query_from = now - datetime.timedelta(minutes=15)

    api_instance = security_monitoring_api.SecurityMonitoringApi(api_client)
    body = SecurityMonitoringSignalListRequest(
        filter=SecurityMonitoringSignalListRequestFilter(
            _from=query_from.isoformat(),
            query=query,
            to=now.isoformat(),
        ),
        page=SecurityMonitoringSignalListRequestPage(
            limit=25,
        ),
        sort=SecurityMonitoringSignalsSort("timestamp"),
    )
    api_response = api_instance.search_security_monitoring_signals(body=body)
    if not api_response["data"]:
        raise LookupError(query)

    return api_response


class App(common.App):
    def __init__(self):
        common.App.__init__(self)

        configuration = Configuration()
        self.api_client = ApiClient(configuration)

    def __exit__(self):
        self.api_client.rest_client.pool_manager.clear()

    def create_cws_signal_rule(self, name, msg, agent_rule_id, tags=None):
        if not tags:
            tags = []

        api_instance = security_monitoring_api.SecurityMonitoringApi(self.api_client)
        body = SecurityMonitoringRuleCreatePayload(
            cases=[
                SecurityMonitoringRuleCaseCreate(
                    condition="a > 0",
                    status=SecurityMonitoringRuleSeverity("info"),
                ),
            ],
            has_extended_title=True,
            is_enabled=True,
            name=name,
            message=msg,
            options=SecurityMonitoringRuleOptions(
                detection_method=SecurityMonitoringRuleDetectionMethod("threshold"),
                evaluation_window=SecurityMonitoringRuleEvaluationWindow(0),
                keep_alive=SecurityMonitoringRuleKeepAlive(0),
                max_signal_duration=SecurityMonitoringRuleMaxSignalDuration(0),
            ),
            queries=[
                # TODO(paulcacheux): maybe change back to SecurityMonitoringRuleQuery
                # once the api client is fixed.
                # 2.4.0 and 2.5.0 are broken because they send `is_enabled` instead
                # of `isEnabled`, resulting in the signal rule being disabled.
                SecurityMonitoringStandardRuleQuery(
                    aggregation=SecurityMonitoringRuleQueryAggregation("count"),
                    query="@agent.rule_id:" + agent_rule_id,
                    name="a",
                ),
            ],
            tags=tags,
            type=SecurityMonitoringRuleTypeCreate("workload_security"),
        )
        response = api_instance.create_security_monitoring_rule(body)
        return response.id

    def create_cws_agent_rule(self, name, msg, secl, tags=None):
        if not tags:
            tags = []

        api_instance = cloud_workload_security_api.CloudWorkloadSecurityApi(self.api_client)
        body = CloudWorkloadSecurityAgentRuleCreateRequest(
            data=CloudWorkloadSecurityAgentRuleCreateData(
                attributes=CloudWorkloadSecurityAgentRuleCreateAttributes(
                    description=msg,
                    enabled=True,
                    expression=secl,
                    name=name,
                ),
                type=CloudWorkloadSecurityAgentRuleType("agent_rule"),
            ),
        )

        api_response = api_instance.create_cloud_workload_security_agent_rule(body)
        return api_response.data.id

    def delete_signal_rule(self, rule_id):
        api_instance = security_monitoring_api.SecurityMonitoringApi(self.api_client)

        try:
            api_instance.delete_security_monitoring_rule(rule_id)
        except ApiException as e:
            print(f"Exception when calling SecurityMonitoringApi->delete_security_monitoring_rule: {e}")

    def delete_agent_rule(self, rule_id):
        api_instance = cloud_workload_security_api.CloudWorkloadSecurityApi(self.api_client)

        try:
            api_instance.delete_cloud_workload_security_agent_rule(rule_id)
        except ApiException as e:
            print(f"Exception when calling CloudWorkloadSecurityApi->delete_cloud_workload_security_agent_rule: {e}")

    def download_policies(self):
        site = os.environ["DD_SITE"]
        api_key = os.environ["DD_API_KEY"]
        app_key = os.environ["DD_APP_KEY"]

        url = f"https://api.{site}/api/v2/remote_config/products/cws/policy/download"
        with requests.get(
            url,
            headers={
                "Content-Type": "application/json",
                "DD-API-KEY": api_key,
                "DD-APPLICATION-KEY": app_key,
            },
            stream=True,
        ) as response:
            with tempfile.NamedTemporaryFile(prefix="e2e-test-", mode="wb", delete=False) as fp:
                for chunk in response.iter_content(chunk_size=4096):
                    fp.write(chunk)

                return fp.name

    def wait_app_log(self, query, tries=30, delay=10):
        return retry_call(get_app_log, fargs=[self.api_client, query], tries=tries, delay=delay)

    def wait_app_signal(self, query, tries=30, delay=10):
        return retry_call(get_app_signal, fargs=[self.api_client, query], tries=tries, delay=delay)

    def check_for_ignored_policies(self, test_case, policies):
        if "policies_ignored" in policies:
            test_case.assertEqual(len(policies["policies_ignored"]), 0)
        if "policies" in policies:
            for policy in policies["policies"]:
                if "rules_ignored" in policy:
                    test_case.assertEqual(len(policy["rules_ignored"]), 0)

    def __find_policy(self, policies, policy_source, policy_name):
        found = False
        if "policies" in policies:
            for policy in policies["policies"]:
                if "source" in policy and "name" in policy:
                    if policy["source"] == policy_source and policy["name"] == policy_name:
                        found = True
                        break
        return found

    def check_policy_found(self, test_case, policies, policy_source, policy_name):
        test_case.assertTrue(
            self.__find_policy(policies, policy_source, policy_name),
            msg=f"should find policy in log (source:{policy_source} name:{policy_name})",
        )

    def check_policy_not_found(self, test_case, policies, policy_source, policy_name):
        test_case.assertFalse(
            self.__find_policy(policies, policy_source, policy_name),
            msg=f"shouldn't find policy in log (source:{policy_source} name:{policy_name})",
        )
