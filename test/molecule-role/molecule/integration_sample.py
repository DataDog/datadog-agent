

def get_agent_integration_sample_expected_topology():
    return [
        {
            "assertion": "Should find the this-host component",
            "type": "Host",
            "external_id": lambda e_id: "urn:example:/host:this_host" == e_id,
            "data": lambda d: d == {
                "checks": [
                    {
                        "critical_value": 90,
                        "deviating_value": 75,
                        "is_metric_maximum_average_check": 1,
                        "max_window": 300000,
                        "name": "Max CPU Usage (Average)",
                        "remediation_hint": "There is too much activity on this host",
                        "stream_id": -1
                    },
                    {
                        "critical_value": 90,
                        "deviating_value": 75,
                        "is_metric_maximum_last_check": 1,
                        "max_window": 300000,
                        "name": "Max CPU Usage (Last)",
                        "remediation_hint": "There is too much activity on this host",
                        "stream_id": -1
                    },
                    {
                        "critical_value": 5,
                        "deviating_value": 10,
                        "is_metric_minimum_average_check": 1,
                        "max_window": 300000,
                        "name": "Min CPU Usage (Average)",
                        "remediation_hint": "There is too few activity on this host",
                        "stream_id": -1
                    },
                    {
                        "critical_value": 5,
                        "deviating_value": 10,
                        "is_metric_minimum_last_check": 1,
                        "max_window": 300000,
                        "name": "Min CPU Usage (Last)",
                        "remediation_hint": "There is too few activity on this host",
                        "stream_id": -1
                    }
                ],
                "domain": "Webshop",
                "environment": "Production",
                "identifiers": [
                    "another_identifier_for_this_host"
                ],
                "labels": [
                    "host:this_host",
                    "region:eu-west-1"
                ],
                "layer": "Machines",
                "metrics": [
                    {
                        "aggregation": "MEAN",
                        "conditions": [
                            {
                                "key": "tags.hostname",
                                "value": "this-host"
                            },
                            {
                                "key": "tags.region",
                                "value": "eu-west-1"
                            }
                        ],
                        "metric_field": "system.cpu.usage",
                        "name": "Host CPU Usage",
                        "priority": "HIGH",
                        "stream_id": -1,
                        "unit_of_measure": "Percentage"
                    },
                    {
                        "aggregation": "MEAN",
                        "conditions": [
                            {
                                "key": "tags.hostname",
                                "value": "this-host"
                            },
                            {
                                "key": "tags.region",
                                "value": "eu-west-1"
                            }
                        ],
                        "metric_field": "location.availability",
                        "name": "Host Availability",
                        "priority": "HIGH",
                        "stream_id": -2,
                        "unit_of_measure": "Percentage"
                    }
                ],
                "name": "this-host",
                "tags": [
                    "integration-type:agent-integration",
                    "integration-url:sample"
                ]
            }
        },
        {
            "assertion": "Should find the some-application component",
            "type": "Application",
            "external_id": lambda e_id: "urn:example:/application:some_application" == e_id,
            "data": lambda d: d == {
                "checks": [
                    {
                        "critical_value": 75,
                        "denominator_stream_id": -1,
                        "deviating_value": 50,
                        "is_metric_maximum_ratio_check": 1,
                        "max_window": 300000,
                        "name": "OK vs Error Responses (Maximum)",
                        "numerator_stream_id": -2
                    },
                    {
                        "critical_value": 70,
                        "deviating_value": 50,
                        "is_metric_maximum_percentile_check": 1,
                        "max_window": 300000,
                        "name": "Error Response 99th Percentile",
                        "percentile": 99,
                        "stream_id": -2
                    },
                    {
                        "critical_value": 75,
                        "denominator_stream_id": -1,
                        "deviating_value": 50,
                        "is_metric_failed_ratio_check": 1,
                        "max_window": 300000,
                        "name": "OK vs Error Responses (Failed)",
                        "numerator_stream_id": -2
                    },
                    {
                        "critical_value": 5,
                        "deviating_value": 10,
                        "is_metric_minimum_percentile_check": 1,
                        "max_window": 300000,
                        "name": "Success Response 99th Percentile",
                        "percentile": 99,
                        "stream_id": -1
                    }
                ],
                "domain": "Webshop",
                "environment": "Production",
                "identifiers": [
                    "another_identifier_for_some_application"
                ],
                "labels": [
                    "application:some_application",
                    "region:eu-west-1",
                    "hosted_on:this-host"
                ],
                "layer": "Applications",
                "metrics": [
                    {
                        "aggregation": "MEAN",
                        "conditions": [
                            {
                                "key": "tags.application",
                                "value": "some_application"
                            },
                            {
                                "key": "tags.region",
                                "value": "eu-west-1"
                            }
                        ],
                        "metric_field": "2xx.responses",
                        "name": "2xx Responses",
                        "priority": "HIGH",
                        "stream_id": -1,
                        "unit_of_measure": "Count"
                    },
                    {
                        "aggregation": "MEAN",
                        "conditions": [
                            {
                                "key": "tags.application",
                                "value": "some_application"
                            },
                            {
                                "key": "tags.region",
                                "value": "eu-west-1"
                            }
                        ],
                        "metric_field": "5xx.responses",
                        "name": "5xx Responses",
                        "priority": "HIGH",
                        "stream_id": -2,
                        "unit_of_measure": "Count"
                    }
                ],
                "name": "some-application",
                "tags": [
                    "integration-type:agent-integration",
                    "integration-url:sample"
                ],
                "version": "0.2.0"
            }
        }
    ]
