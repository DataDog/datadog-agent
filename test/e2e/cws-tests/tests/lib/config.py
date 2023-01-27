import tempfile

import yaml


def gen_system_probe_config(npm_enabled=False, rc_enabled=False, log_level="INFO", log_patterns=None):
    fp = tempfile.NamedTemporaryFile(prefix="e2e-system-probe-", mode="w", delete=False)

    if not log_patterns:
        log_patterns = []

    data = {
        "system_probe_config": {"log_level": log_level},
        "network_config": {"enabled": npm_enabled},
        "runtime_security_config": {
            "log_patterns": log_patterns,
            "network": {"enabled": True},
            "remote_configuration": {"enabled": rc_enabled, "refresh_interval": "5s"},
        },
    }

    yaml.dump(data, fp)
    fp.close()

    return fp.name


def gen_datadog_agent_config(hostname="myhost", log_level="INFO", tags=None, rc_enabled=False, rc_key=None):
    fp = tempfile.NamedTemporaryFile(prefix="e2e-datadog-agent-", mode="w", delete=False)

    if not tags:
        tags = []

    data = {
        "log_level": log_level,
        "hostname": hostname,
        "tags": tags,
        "security_agent.remote_workloadmeta": True,
        "remote_configuration": {"enabled": rc_enabled, "refresh_interval": "5s", "key": rc_key},
    }

    yaml.dump(data, fp)
    fp.close()

    return fp.name
