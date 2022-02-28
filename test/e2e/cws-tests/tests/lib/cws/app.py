import os
import tempfile

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
    SecurityMonitoringRuleQueryCreate,
    SecurityMonitoringRuleSeverity,
    SecurityMonitoringRuleTypeCreate,
    SecurityMonitoringSignalListRequest,
    SecurityMonitoringSignalListRequestFilter,
    SecurityMonitoringSignalListRequestPage,
    SecurityMonitoringSignalsSort,
)
from dateutil.parser import parse as dateutil_parser
from retry.api import retry_call


def get_app_log(api_client, query):
    api_instance = logs_api.LogsApi(api_client)
    body = LogsListRequest(
        filter=LogsQueryFilter(
            _from="now-15m",
            indexes=["main"],
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
    api_instance = security_monitoring_api.SecurityMonitoringApi(api_client)
    body = SecurityMonitoringSignalListRequest(
        filter=SecurityMonitoringSignalListRequestFilter(
            _from=dateutil_parser("2021-01-01T00:00:00.00Z"),
            query=query,
            to=dateutil_parser("2050-01-01T00:00:00.00Z"),
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


class App:
    def __init__(self):
        configuration = Configuration()
        configuration.unstable_operations["search_security_monitoring_signals"] = True

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
                SecurityMonitoringRuleQueryCreate(
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
        # TODO: launch the downlaod directly from the docker/kube-pod?
        dd_security_agent_path = os.getenv('DD_SECURITY_AGENT_PATH')
        if not dd_security_agent_path:
            dd_security_agent_path = "./bin/security-agent/security-agent"
        policy = tempfile.NamedTemporaryFile(prefix="e2e-test-", mode="wb", delete=False)
        dl_cmd = dd_security_agent_path + " runtime policy download --output-path " + policy.name + " 2>/dev/null"
        os.system(dl_cmd)
        return policy.name

    def wait_app_log(self, query, tries=30, delay=10):
        return retry_call(get_app_log, fargs=[self.api_client, query], tries=tries, delay=delay)

    def wait_app_signal(self, query, tries=30, delay=10):
        return retry_call(get_app_signal, fargs=[self.api_client, query], tries=tries, delay=delay)
