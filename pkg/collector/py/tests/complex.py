# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

d = {
    'init_config': {
        'conf': [
            {
                'include': {
                    'attribute': {
                        'Count': {'alias': 'kafka.net.bytes_out.rate', 'metric_type': 'rate'}
                    },
                    'bean': 'kafka.server:type=BrokerTopicMetrics,name=BytesOutPerSec',
                    'domain': 'kafka.server'
                }
            }
        ],
        'is_jmx': True
    },
    'instances': [
        {'host': 'localhost', 'port': 9999, 'tags': {'env': 'stage', 'newTag': 'test'}},
        {'host': 'otherhost', 'port': 1234}
    ]
}
