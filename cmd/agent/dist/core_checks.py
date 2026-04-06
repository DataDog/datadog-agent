"""
Agent checks. This file is both checks.py and checks.bzl
"""

AGENT_CORECHECKS = [
    "container",
    "containerd",
    "container_image",
    "container_lifecycle",
    "cpu",
    "cri",
    "snmp",
    "docker",
    "file_handle",
    "go_expvar",
    "io",
    "jmx",
    "kubernetes_apiserver",
    "load",
    "memory",
    "ntp",
    "oom_kill",
    "oracle",
    "oracle-dbm",
    "sbom",
    "systemd",
    "tcp_queue_length",
    "uptime",
    "jetson",
    "telemetry",
    "orchestrator_pod",
    "orchestrator_kubelet_config",
    "orchestrator_ecs",
    "cisco_sdwan",
    "network_path",
    "gpu",
    "wlan",
    "discovery",
    "versa",
    "network_config_management",
    "battery",
    "cloud_hostinfo",
]

WINDOWS_CORECHECKS = [
    "agentcrashdetect",
    "sbom",
    "windows_registry",
    "winkmem",
    "wincrashdetect",
    "windows_certificate",
    "winproc",
    "win32_event_log",
]

IOT_AGENT_CORECHECKS = [
    "cpu",
    "disk",
    "io",
    "load",
    "memory",
    "network",
    "ntp",
    "uptime",
    "systemd",
    "jetson",
]

CACHED_WHEEL_FILENAME_PATTERN = "datadog_{integration}-*.whl"
CACHED_WHEEL_DIRECTORY_PATTERN = "integration-wheels/{branch}/{hash}/{python_version}/"
CACHED_WHEEL_FULL_PATH_PATTERN = CACHED_WHEEL_DIRECTORY_PATTERN + CACHED_WHEEL_FILENAME_PATTERN
LAST_DIRECTORY_COMMIT_PATTERN = "git -C {integrations_dir} rev-list -1 HEAD {integration}"
