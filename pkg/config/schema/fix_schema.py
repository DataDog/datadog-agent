#!/usr/bin/env python

import yaml
import sys

"""
This script fixes the OS differences in the schema that are hard to capture automatically. This includes information
decided at runtime by the Agent, OS based description and more.
"""

# Available Variables
#
# Most of those can be change by customers and are resolved at runtime when the Agent start. The following are the most
# likely values.
#
# conf_path:
#     linux: /etc/datadog-agent/
#     windows: programdata Dir, likely c:\programdata\datadog
#     darwin: /opt/datadog-agent/etc/
# install_path:
#     linux: /opt/datadog-agent/
#     windows: likely c:\Program Files\Datadog\Datadog Agent
#     darwin: /opt/datadog-agent
# run_path:
#    linux: {install_path}/run
#    darwin: /opt/datadog-agent/run
#    windows: {conf_path}\run
# log_path:
#     linux: /var/log/datadog
#     darwin: /opt/datadog-agent/logs
#     windows: c:\programdata\datadog\logs

core_defaults = {
    "sbom.cache_directory": "${run_path}/sbom-agent",
    "agent_ipc.socket_path": "${run_path}/agent_ipc.socket",
    "logs_config.open_files_limit": {
        "darwin": 200,
        "other": 500,
        },

    "use_networkv2_check": {
        "darwin": False,
        "other": True
        },
    "network_check.use_core_loader": {
        "darwin": False,
        "other": True
        },

    "kubernetes_kubelet_podresources_socket": {
        "windows": "\\\\.\\pipe\\kubelet-pod-resources",
        "other": "/var/lib/kubelet/pod-resources/kubelet.sock",
        },
    "kubernetes_kubelet_deviceplugins_socketdir": {
        "windows": "\\\\.\\pipe\\kubelet-device-plugins",
        "other": "/var/lib/kubelet/device-plugins",
        },

    "process_config.run_in_core_agent.enabled": {
        "linux": True,
        "other": False,
        },

    "logs_config.run_path": "${run_path}",
    "run_path": "${run_path}",

    "process_config.dd_agent_bin": {
        "linux": "${install_path}/bin/agent/agent",
        "darwin": "${install_path}/bin/agent/agent",
        "windows": "${install_path}/bin/agent.exe",
        },
    "confd_path": "${conf_path}/conf.d",
    "shared_library_check.library_folder_path": "${conf_path}/checks.d",
    "additional_checksd": "${conf_path}/checks.d",
    "GUI_port": {
            "linux": -1,
            "other": 5002,
            },
    "security_agent.log_file": "${log_path}/security-agent.log",
    "process_config.log_file": "${log_path}/process-agent.log",
    "private_action_runner.log_file": "${log_path}/private-action-runner.log",
    "dogstatsd_socket": {
            "linux": "/var/run/datadog/dsd.socket",
            "other": "",
            },
    "apm_config.receiver_socket": {
            "linux": "/var/run/datadog/apm.socket",
            "other": "",
            },
    "logs_config.streaming.streamlogs_log_file": "${log_path}/streamlogs_info/streamlogs.log",

    "system_tray.log_file": "${conf_path}/logs/ddtray.log",

     # setting duplicated for some reasons between system-probe and core-agent config
    "runtime_security_config.socket": {
        "windows": "localhost:3335",
        "other": "${install_path}/run/runtime-security.sock",
        },
}

sysprobe_defaults = {
    "log_file": "${log_path}/system-probe.log",
    "system_probe_config.bpf_dir": "${install_path}/embedded/share/system-probe/ebpf",
    "system_probe_config.process_service_inference.enabled": {
        "windows": True,
        "other": False,
        },
    "system_probe_config.sysprobe_socket": {
        "linux": "${run_path}/sysprobe.sock",
        "darwin": "/opt/datadog-agent/run/sysprobe.sock",
        "windows": "\\\\.\\pipe\\dd_system_probe",
        },
    "runtime_security_config.security_profile.dir": "${run_path}/runtime-security/profiles",
    "runtime_security_config.activity_dump.local_storage.output_directory": "${run_path}/runtime-security/profiles",
    "runtime_security_config.policies.dir": {
        "windows": "${conf_path}/runtime-security.d",
        "other": "/etc/datadog-agent/runtime-security.d", # hardcoded for both, probably doesn't work on darwin
    },

     # setting duplicated for some reasons between system-probe and core-agent config
    "runtime_security_config.socket": {
        "windows": "localhost:3335",
        "other": "${install_path}/run/runtime-security.sock",
        },
}

# Fix os based descriptions

core_description = {
    "apm_config.receiver_socket": {
        "windows": "Please note that UDS receiver is not available in Windows.\n@ Enabling this setting may result in unexpected behavior.\n\nAccept traces through Unix Domain Sockets.\nSet to \"\" to disable the UDS receiver.",
        "other": "Accept traces through Unix Domain Sockets.\nSet to \"\" to disable the UDS receiver.",
        },
    "runtime_security_config.socket": {
        "windows": "The local address and port where the security runtime module is accessed",
        "other": "The full path to the location of the unix socket where security runtime module is accessed.",
        },
    "dogstatsd_socket": {
        "windows": "Please note that UDS receiver is not available in Windows.\n@ Enabling this setting may result in unexpected behavior.\nListen for Dogstatsd metrics on a Unix Socket (*nix only).\nSet to \"\" to disable this feature.",
        "other": "Listen for Dogstatsd metrics on a Unix Socket (*nix only).\nSet to a valid and existing filesystem path to enable.\nSet to \"\" to disable this feature.",
        },
}

system_probe_description = {
    "system_probe_config.sysprobe_socket": {
        "windows": "The TCP address where system probes are accessed.",
        "other": "The full path to the location of the unix socket where system probes are accessed.",
        },
    "runtime_security_config.socket": {
        "windows": "The TCP address where the security runtime module is accessed.",
        "other": "The full path to the location of the unix socket where security runtime module is accessed.",
        },
    # The templates has the same section twice to account for different defaults. We discard it since we generate
    # defaults from the code values.
    "runtime_security_config.policies.dir": "Path from where the policy files are loaded",
}

# extra_tags

core_extra_tags = {
    "system_tray": ["platform_only:windows"],
    "sbom.container_image": ["platform_only:linux"],
    "network_path": ["platform_only:windows,linux"],
}

system_probe_extra_tags = {
    "windows_crash_detection": ["platform_only:windows"],
}

# fix custom env vars
#
# Some env vars had handled manually by custom code instead of the config

core_extra_env = {
    "proxy.https": "@env DD_PROXY_HTTPS - string - optional - default: \"\"",
    "proxy.http": "@env DD_PROXY_HTTP - string - optional - default: \"\"",
    "proxy.no_proxy": "@env DD_PROXY_NO_PROXY - space separated list of strings - optional - default: []",
}

def fix_defaults(core_schema, sysprobe_schema):
    for schema, custom_defaults in [[core_schema, core_defaults], [sysprobe_schema, sysprobe_defaults]]:

        for key, default in custom_defaults.items():
            node = schema
            for k in key.split("."):
                node = node["properties"][k]

            if isinstance(default, str):
                node["default"] = default
            elif isinstance(default, dict):
                if "default" in node:
                    del node["default"]
                node["platform_default"] = default
            else:
                panic(f"unknown custom default type {type(default)} for {key}")
    return core_schema, sysprobe_schema


def fix_description(core_schema, sysprobe_schema):
    for schema, custom_description in [[core_schema, core_description], [sysprobe_schema, system_probe_description]]:

        for key, description in custom_description.items():
            node = schema
            for k in key.split("."):
                node = node["properties"][k]

            node["description"] = description
            if "example" in node:
                del node["example"]
    return core_schema, sysprobe_schema

def fix_tags(core_schema, sysprobe_schema):
    for schema, new_tags in [[core_schema, core_extra_tags], [sysprobe_schema, system_probe_extra_tags]]:

        for key, tags in new_tags.items():
            node = schema
            for k in key.split("."):
                node = node["properties"][k]

            node["tags"] = list(set(node.get("tags", []) + tags))
    return core_schema, sysprobe_schema

def fix_missing_env_doc(core_schema, sysprobe_schema):
    # no extra env for sysprobe
    for schema, env_lines in [[core_schema, core_extra_env]]:

        for key, line in env_lines.items():
            node = schema
            for k in key.split("."):
                node = node["properties"][k]

            node["description"] = line +"\n"+node.get("description", "")
    return core_schema, sysprobe_schema

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} core_schema.yaml sysprobe_schema.yaml")
        sys.exit(1)

    with open(sys.argv[1], "r") as f:
        core_schema = yaml.safe_load(f)
    with open(sys.argv[2], "r") as f:
        sysprobe_schema = yaml.safe_load(f)

    core_schema, sysprobe_schema = fix_defaults(core_schema, sysprobe_schema)
    core_schema, sysprobe_schema = fix_description(core_schema, sysprobe_schema)
    core_schema, sysprobe_schema = fix_tags(core_schema, sysprobe_schema)
    core_schema, sysprobe_schema = fix_missing_env_doc(core_schema, sysprobe_schema)

    with open(sys.argv[1], "w") as f:
        yaml.safe_dump(core_schema, f)
    with open(sys.argv[2], "w") as f:
        yaml.safe_dump(sysprobe_schema, f)
