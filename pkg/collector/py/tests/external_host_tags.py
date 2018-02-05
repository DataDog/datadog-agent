import datadog_agent

tags = [
    (
        'test-py-localhost',
        {
            'test-source-type': [
                'tag1',
                'tag2',
                'tag3',
            ]
        }
    )
]

def test():
    datadog_agent.add_external_tags(tags)
