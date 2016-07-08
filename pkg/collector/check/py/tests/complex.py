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
