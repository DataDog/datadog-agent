# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

from checks import AgentCheck, TopologyInstance
import time

class TestTopologyEvents(AgentCheck):
    def get_instance_key(self, instance):
        return TopologyInstance("type", "url")

    def check(self, instance):
        # gets the value of the `url` property
        instance_url = instance.get('url', 'agent-integration-sample')
        # gets the value of the `default_timeout` property or defaults to 5
        default_timeout = self.init_config.get('default_timeout', 5)
        # gets the value of the `timeout` property or defaults `default_timeout` and casts it to a float data type
        timeout = float(instance.get('timeout', default_timeout))

        self.event({
            "timestamp": int(time.time()),
            "source_type_name": "HTTP_TIMEOUT",
            "msg_title": "URL timeout",
            "msg_text": "Http request to %s timed out after %s seconds." % (instance_url, timeout),
            "aggregation_key": "instance-request-%s" % instance_url,
            "context": {
                "source_identifier": "source_identifier_value",
                "element_identifiers": ["urn:host:/123"],
                "source": "source_value",
                "category": "my_category",
                "data": {"big_black_hole": "here"},
                "source_links": [
                    {"title": "my_event_external_link", "url": "http://localhost"}
                ]
            }
        })
