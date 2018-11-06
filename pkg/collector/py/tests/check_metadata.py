import datadog_agent

METADATA_PAYLOADS = (
    ('http_check:12345', 'config.http_check.exists.check_certificate_expiration', 'true'),
    ('redis:12345', 'version.redis', '4.0.0'),
    ('redis:12345', 'version.redis', '5.0.0'),
)


def test():
    for check_id, name, value in METADATA_PAYLOADS:
        datadog_agent.set_check_metadata(check_id, name, value)
