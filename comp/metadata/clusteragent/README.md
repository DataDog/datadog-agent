# Cluster-agent Metadata Payload
This package populates some of the cluster-agent related fields in the cluster-agent (DDSQL) table.

This is disabled by default now but can be accessible to flare (cluster-agent-metadata.json) and endpoint of /metadata/cluster-agent.
If enable_cluster_agent_metadata_collection is enabled, the payload is also sent to backend every 10 minutes (see inventories_max_interval in the config).

# Cluster-agent Configuration
The agent configurations are scrubbed from any sensitive information (same logic as for the flare). The Format section goes into more detail about what configuration is sent.

Sending Cluster-Agent configuration can be disabled using enable_cluster_agent_metadata_collection.

# Format
The payload is a JSON dictionary with the following fields:

- `clustername` - string: the name of the cluster.
- `cluster_id` - string: the unique identifier of the cluster.
- `timestamp` - int: the timestamp when the payload was created.
- `datadog_cluster_agent_metadata` - dict of string to JSON type:
- `agent_version` - string: the version of the Agent sending this payload.
- `full_configuration` - string: the current Cluster-Agent configuration scrubbed, including all the defaults, as a YAML string.
- `provided_configuration` - string: the current Cluster-Agent configuration (scrubbed), without the defaults, as a YAML string. This includes the settings configured by the user (through the configuration file, the environment, CLI, etc.).
- `file_configuration` - string: the Cluster-Agent configuration specified by the configuration file (scrubbed), as a YAML string. Only the settings written in the configuration file are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
- `environment_variable_configuration` - string: the Cluster-Agent configuration specified by the environment variables (scrubbed), as a YAML string. Only the settings written in the environment variables are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
- `agent_runtime_configuration` - string: the Cluster-Agent configuration set by the agent itself (scrubbed), as a YAML string. Only the settings set by the agent itself are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
remote_configuration - string: the Cluster-Agent configuration specified by the Remote Configuration (scrubbed), as a YAML string. Only the settings currently used by Remote Configuration are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
- `fleet_policies_configuration` - string: the Cluster-Agent configuration specified by the Fleet Automation Policies (scrubbed), as a YAML string. Only the settings currently used by Fleet Automation Policies are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
- `cli_configuration` - string: the Cluster-Agent configuration specified by the CLI (scrubbed), as a YAML string. Only the settings set in the CLI are included.
source_local_configuration - string: the Cluster-Agent configuration synchronized from the local Agent process, as a YAML string.
- `agent_startup_time_ms` - int: the startup time of the agent in milliseconds.
- `cluster_id_error` - string: any error related to the cluster ID.
- feature_*` - various types: various feature flags and their statuses.

("scrubbed" indicates that secrets are removed from the field value just as they are in logs)

## Example Payload
```
{
    "clustername": "test-gke",
    "cluster_id": "d1bd4888-2990-4312-a610-c9eae75acba4",
    "timestamp": 1742810761534538592,
    "datadog_cluster_agent_metadata": {
        "agent_runtime_configuration": "internal_profiling:\n  enabled: true\n",
        "agent_startup_time_ms": 1742809996230,
        "agent_version": "7.65.0-devel+git.651.84a74a5",
        "cli_configuration": "{}\n",
        "cluster_id_error": "",
        "environment_variable_configuration": "admission_controller.container_registry: gcr.io/datadoghq\nadmission_controller.enabled: \"true\"\nadmission_controller.failure_policy: Ignore\nadmission_controller.inject_config.local_service_name: datadog-agent-linux\nadmission_controller.inject_config.mode: socket\nadmission_controller.mutate_unlabelled: \"false\"\nadmission_controller.mutation.enabled: \"true\"\nadmission_controller.port: \"8000\"\nadmission_controller.service_name: datadog-agent-linux-cluster-agent-admission-controller\nadmission_controller.validation.enabled: \"true\"\nadmission_controller.webhook_name: datadog-webhook\napi_key: '***************************658e7'\napm_config.install_id: 817926d8-f346-487c-a5bb-c27aa73dfc0b\napm_config.install_time: \"1742809959\"\napm_config.install_type: k8s_manual\nautoscaling.failover.enabled: \"true\"\ncluster_agent.auth_token: '********'\ncluster_agent.collect_kubernetes_tags: \"true\"\ncluster_agent.kubernetes_service_name: datadog-agent-linux-cluster-agent\ncluster_agent.language_detection.patcher.enabled: \"false\"\ncluster_agent.service_account_name: datadog-agent-linux-cluster-agent\ncluster_agent.token_name: datadog-agent-linuxtoken\ncluster_checks.enabled: \"true\"\ncluster_name: minyi-test-gke\ncollect_kubernetes_events: \"true\"\nextra_config_providers: kube_endpoints kube_services\nextra_listeners: kube_endpoints kube_services\nhealth_port: \"5556\"\ninternal_profiling.enabled: \"true\"\nkube_resources_namespace: datadog-agent-helm\nkubernetes_events_source_detection.enabled: \"false\"\nkubernetes_namespace_labels_as_tags: '{\"kubernetes.io/metadata.name\":\"name\"}'\nkubernetes_use_endpoint_slices: \"false\"\nlanguage_detection.enabled: \"false\"\nlanguage_detection.reporting.enabled: \"false\"\nleader_election: \"true\"\nleader_election_default_resource: configmap\nleader_lease_duration: \"15\"\nleader_lease_name: datadog-agent-linux-leader-election\nlog_level: INFO\norchestrator_explorer.container_scrubbing.enabled: \"true\"\norchestrator_explorer.enabled: \"true\"\nproxy:\n  http: \"\"\n  https: \"\"\n  no_proxy:\n  - 169.254.169.254\n  - 100.100.100.200\npython_version: \"3\"\nremote_configuration.enabled: \"false\"\nsecret_backend_command_allow_group_exec_perm: \"true\"\nsecurity_agent.internal_profiling.api_key: '***************************658e7'\nsslkeylogfile: /tmp/sslkeylog.txt\n",
        "feature_admission_controller_auto_instrumentation_enabled": true,
        "feature_admission_controller_cws_instrumentation_enabled": false,
        "feature_admission_controller_enabled": true,
        "feature_admission_controller_inject_config_enabled": true,
        "feature_admission_controller_inject_tags_enabled": true,
        "feature_admission_controller_mutation_enabled": true,
        "feature_admission_controller_validation_enabled": true,
        "feature_apm_config_instrumentation_enabled": false,
        "feature_autoscaling_workload_enabled": false,
        "feature_cluster_checks_advanced_dispatching_enabled": false,
        "feature_cluster_checks_enabled": true,
        "feature_cluster_checks_exclude_checks": [],
        "feature_compliance_config_enabled": false,
        "feature_external_metrics_provider_enabled": false,
        "feature_external_metrics_provider_use_datadogmetric_crd": false,
        "file_configuration": "{}\n",
        "flavor": "cluster_agent",
        "fleet_policies_configuration": "{}\n",
        "full_configuration": "ac_exclude: xxxx",
        "install_method_installer_version": "datadog-3.110.2",
        "install_method_tool": "helm",
        "install_method_tool_version": "Helm",
        "is_leader": true,
        "leader_election": true,
        "provided_configuration": "admission_controller ***",
        "remote_configuration": "{}\n",
        "source_local_configuration": "{}\n"
    },
    "uuid": "05284024-b312-f0ac-4769-9e9b99baf163"
}
```