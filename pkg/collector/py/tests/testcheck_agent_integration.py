# (C) StackState 2020
# All rights reserved
# Licensed under a 3-clause BSD style license (see LICENSE)

# project
from stackstate_checks.base import AgentCheck, AgentIntegrationInstance, MetricStream, MetricHealthChecks
import time
from random import seed
from random import randint

seed(1)


class AgentIntegrationSampleCheck(AgentCheck):
    def get_instance_key(self, instance):
        return AgentIntegrationInstance("type", "url")

    def check(self, instance):
        # gets the value of the `url` property
        instance_url = instance.get('url', 'agent-integration-sample')
        # gets the value of the `default_timeout` property or defaults to 5
        default_timeout = self.init_config.get('default_timeout', 5)
        # gets the value of the `timeout` property or defaults `default_timeout` and casts it to a float data type
        timeout = float(instance.get('timeout', default_timeout))

        this_host_cpu_usage = MetricStream("Host CPU Usage", "host.cpu.usage",
                                           conditions={"tags.hostname": "this-host"},
                                           unit_of_measure="Percentage",
                                           aggregation="MEAN",
                                           priority="HIGH")
        cpu_max_average_check = MetricHealthChecks.maximum_average(this_host_cpu_usage.identifier,
                                                                   "Max CPU Usage (Average)", 75, 90)
        cpu_max_last_check = MetricHealthChecks.maximum_last(this_host_cpu_usage.identifier,
                                                             "Max CPU Usage (Last)", 75, 90)
        cpu_min_average_check = MetricHealthChecks.minimum_average(this_host_cpu_usage.identifier,
                                                                   "Min CPU Usage (Average)", 10, 5)
        cpu_min_last_check = MetricHealthChecks.minimum_last(this_host_cpu_usage.identifier,
                                                             "Min CPU Usage (Last)", 10, 5)
        self.component("urn:example:/host:this_host", "Host",
                       data={
                           "name": "this-host",
                           "domain": "Webshop",
                           "layer": "Hosts",
                           "identifiers": ["another_identifier_for_this_host"],
                           "labels": ["host:this_host", "region:eu-west-1"],
                           "environment": "Production"
                       },
                       streams=[this_host_cpu_usage],
                       checks=[cpu_max_average_check, cpu_max_last_check, cpu_min_average_check, cpu_min_last_check])

        self.gauge("host.cpu.usage", randint(0, 100), tags=["hostname:this-host"])
        self.gauge("host.cpu.usage", randint(0, 100), tags=["hostname:this-host"])
        self.gauge("host.cpu.usage", randint(0, 100), tags=["hostname:this-host"])
        self.gauge("host.cpu.usage", randint(0, 100), tags=["hostname:this-host"])
        self.gauge("host.cpu.usage", randint(0, 100), tags=["hostname:this-host"])
        self.gauge("host.cpu.usage", randint(0, 100), tags=["hostname:this-host"])
        self.gauge("host.cpu.usage", randint(0, 100), tags=["hostname:this-host"])

        some_application_2xx_responses = MetricStream("2xx Responses", "2xx.responses",
                                                      conditions={"tags.application": "some_application",
                                                                  "tags.region": "eu-west-1"},
                                                      unit_of_measure="Count",
                                                      aggregation="MEAN",
                                                      priority="HIGH")
        some_application_5xx_responses = MetricStream("5xx Responses", "5xx.responses",
                                                      conditions={"tags.application": "some_application",
                                                                  "tags.region": "eu-west-1"},
                                                      unit_of_measure="Count",
                                                      aggregation="MEAN",
                                                      priority="HIGH")
        max_response_ratio_check = MetricHealthChecks.maximum_ratio(some_application_2xx_responses.identifier,
                                                                    some_application_5xx_responses.identifier,
                                                                    "OK vs Error Responses (Maximum)",
                                                                    50, 75)
        max_percentile_response_check = MetricHealthChecks.maximum_percentile(some_application_5xx_responses.identifier,
                                                                              "Error Response 99th Percentile",
                                                                              50, 70, 99)
        failed_response_ratio_check = MetricHealthChecks.failed_ratio(some_application_2xx_responses.identifier,
                                                                      some_application_5xx_responses.identifier,
                                                                      "OK vs Error Responses (Failed)",
                                                                      50, 75)
        min_percentile_response_check = MetricHealthChecks.minimum_percentile(some_application_2xx_responses.identifier,
                                                                              "Success Response 99th Percentile",
                                                                              10, 5, 99)
        self.component("urn:example:/application:some_application", "Application",
                       data={
                           "name": "some-application",
                           "domain": "Webshop",
                           "layer": "Applications",
                           "identifiers": ["another_identifier_for_some_application"],
                           "labels": ["application:some_application", "region:eu-west-1", "hosted_on:this-host"],
                           "environment": "Production",
                           "version": "0.2.0"
                       },
                       streams=[some_application_2xx_responses, some_application_5xx_responses],
                       checks=[max_response_ratio_check, max_percentile_response_check, failed_response_ratio_check,
                               min_percentile_response_check])

        self.relation("urn:example:/application:some_application", "urn:example:/host:this_host", "IS_HOSTED_ON", {})

        self.gauge("2xx.responses", randint(0, 100), tags=["application:some_application", "region:eu-west-1"])
        self.gauge("2xx.responses", randint(0, 100), tags=["application:some_application", "region:eu-west-1"])
        self.gauge("2xx.responses", randint(0, 100), tags=["application:some_application", "region:eu-west-1"])
        self.gauge("2xx.responses", randint(0, 100), tags=["application:some_application", "region:eu-west-1"])
        self.gauge("5xx.responses", randint(0, 100), tags=["application:some_application", "region:eu-west-1"])
        self.gauge("5xx.responses", randint(0, 100), tags=["application:some_application", "region:eu-west-1"])
        self.gauge("5xx.responses", randint(0, 100), tags=["application:some_application", "region:eu-west-1"])
        self.gauge("5xx.responses", randint(0, 100), tags=["application:some_application", "region:eu-west-1"])

        self.event({
            "timestamp": int(time.time()),
            "source_type_name": "HTTP_TIMEOUT",
            "msg_title": "URL timeout",
            "msg_text": "Http request to %s timed out after %s seconds." % (instance_url, timeout),
            "aggregation_key": "instance-request-%s" % instance_url
        })

        # some logic here to test our connection and if successful:
        self.service_check("example.can_connect", AgentCheck.OK, tags=["instance_url:%s" % instance_url])
