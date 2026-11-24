# Cluster-agent Metadata Payload
This package populates some of the cluster-agent related fields in the cluster-agent (DDSQL) table.

This is enabled by default (`enable_cluster_agent_metadata_collection` is `true`): the payload is sent to the backend every 10 minutes (see `inventories_max_interval` in the config).
If `enable_cluster_agent_metadata_collection` is disabled, the payload is not sent to the backend but is still accessible through the flare (`cluster-agent-metadata.json`) and the `/metadata/cluster-agent` endpoint.

# Cluster-agent Configuration
The agent configurations are scrubbed from any sensitive information (same logic as for the flare). The Format section goes into more detail about what configuration is sent.

Sending Cluster-Agent configuration and the `feature_*` fields can be disabled using `inventories_configuration_enabled`.

# Format
The payload is a JSON dictionary with the following fields:

- `clustername` - string: the name of the cluster.
- `cluster_id` - string: the unique identifier of the cluster.
- `timestamp` - int: the timestamp when the payload was created.
- `uuid` - string: a unique identifier of the host running the Cluster-Agent.
- `datadog_cluster_agent_metadata` - dict of string to JSON type:
  - `agent_version` - string: the version of the Agent sending this payload.
  - `agent_startup_time_ms` - int: the startup time of the agent in milliseconds.
  - `flavor` - string: the flavor of the Agent (`cluster_agent`).
  - `install_method_tool` - string: the tool used to install the Cluster-Agent (ex: `helm`, `operator`), `undefined` if unknown.
  - `install_method_tool_version` - string: the version of the tool used to install the Cluster-Agent.
  - `install_method_installer_version` - string: the version of the installer (ex: the Helm chart version).
  - `pod_name` - string: the name of the Cluster-Agent pod, empty if it can't be determined.
  - `cluster_id_error` - string: any error related to the cluster ID.
  - `leader_election` - boolean: the value of the `leader_election` configuration.
  - `is_leader` - boolean: true if this Cluster-Agent is the leader (always false when leader election is disabled).
  - `cluster_check_runner_count` - int: the number of cluster check runners. Only sent when cluster checks are running.
  - `cluster_check_node_agent_count` - int: the number of node Agents. Only sent when cluster checks are running.
  - `full_configuration` - string: the current Cluster-Agent configuration scrubbed, including all the defaults, as a YAML string.
  - `provided_configuration` - string: the current Cluster-Agent configuration (scrubbed), without the defaults, as a YAML string. This includes the settings configured by the user (through the configuration file, the environment, CLI, etc.).
  - `file_configuration` - string: the Cluster-Agent configuration specified by the configuration file (scrubbed), as a YAML string. Only the settings written in the configuration file are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
  - `environment_variable_configuration` - string: the Cluster-Agent configuration specified by the environment variables (scrubbed), as a YAML string. Only the settings written in the environment variables are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
  - `agent_runtime_configuration` - string: the Cluster-Agent configuration set by the agent itself (scrubbed), as a YAML string. Only the settings set by the agent itself are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
  - `remote_configuration` - string: the Cluster-Agent configuration specified by the Remote Configuration (scrubbed), as a YAML string. Only the settings currently used by Remote Configuration are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
  - `fleet_policies_configuration` - string: the Cluster-Agent configuration specified by the Fleet Automation Policies (scrubbed), as a YAML string. Only the settings currently used by Fleet Automation Policies are included, and their value might not match what's applied by the agent since they can be overridden by other sources.
  - `cli_configuration` - string: the Cluster-Agent configuration specified by the CLI (scrubbed), as a YAML string. Only the settings set in the CLI are included.
  - `source_local_configuration` - string: the Cluster-Agent configuration synchronized from the local Agent process, as a YAML string.
  - `feature_admission_controller_enabled` - boolean: `admission_controller.enabled`.
  - `feature_admission_controller_inject_config_enabled` - boolean: `admission_controller.inject_config.enabled`.
  - `feature_admission_controller_inject_tags_enabled` - boolean: `admission_controller.inject_tags.enabled`.
  - `feature_admission_controller_validation_enabled` - boolean: `admission_controller.validation.enabled`.
  - `feature_admission_controller_mutation_enabled` - boolean: `admission_controller.mutation.enabled`.
  - `feature_admission_controller_auto_instrumentation_enabled` - boolean: `admission_controller.auto_instrumentation.enabled`.
  - `feature_admission_controller_cws_instrumentation_enabled` - boolean: `admission_controller.cws_instrumentation.enabled`.
  - `feature_apm_config_instrumentation_enabled` - boolean: `apm_config.instrumentation.enabled`.
  - `feature_autoscaling_workload_enabled` - boolean: `autoscaling.workload.enabled`.
  - `feature_autoscaling_cluster_enabled` - boolean: `autoscaling.cluster.enabled`.
  - `feature_remote_configuration_enabled` - boolean: `remote_configuration.enabled`.
  - `feature_external_metrics_provider_enabled` - boolean: `external_metrics_provider.enabled`.
  - `feature_external_metrics_provider_use_datadogmetric_crd` - boolean: `external_metrics_provider.use_datadogmetric_crd`.
  - `feature_compliance_config_enabled` - boolean: `compliance_config.enabled`.
  - `feature_cluster_checks_enabled` - boolean: `cluster_checks.enabled`.
  - `feature_cluster_checks_advanced_dispatching_enabled` - boolean: `cluster_checks.advanced_dispatching_enabled`.
  - `feature_cluster_checks_exclude_checks` - list of string: `cluster_checks.exclude_checks`.

The `*_configuration` and `feature_*` fields are only sent when `inventories_configuration_enabled` is true.

("scrubbed" indicates that secrets are removed from the field value just as they are in logs)

## Example Payload
```json
{
    "clustername": "my-cluster",
    "cluster_id": "00000000-0000-0000-0000-000000000002",
    "timestamp": 1742810761534538592,
    "datadog_cluster_agent_metadata": {
        "agent_runtime_configuration": "internal_profiling:\n  enabled: true\n",
        "agent_startup_time_ms": 1742809996230,
        "agent_version": "7.65.0-devel+git.651.84a74a5",
        "cli_configuration": "{}\n",
        "cluster_check_node_agent_count": 3,
        "cluster_check_runner_count": 2,
        "cluster_id_error": "",
        "environment_variable_configuration": "admission_controller.container_registry: gcr.io/datadoghq\nadmission_controller.enabled: \"true\"\nadmission_controller.failure_policy: Ignore\nadmission_controller.inject_config.local_service_name: datadog-agent-linux\nadmission_controller.inject_config.mode: socket\nadmission_controller.mutate_unlabelled: \"false\"\nadmission_controller.mutation.enabled: \"true\"\nadmission_controller.port: \"8000\"\nadmission_controller.service_name: datadog-agent-linux-cluster-agent-admission-controller\nadmission_controller.validation.enabled: \"true\"\nadmission_controller.webhook_name: datadog-webhook\napi_key: '***************************aaaaa'\napm_config.install_id: 00000000-0000-0000-0000-000000000003\napm_config.install_time: \"1742809959\"\napm_config.install_type: k8s_manual\nautoscaling.failover.enabled: \"true\"\ncluster_agent.auth_token: '********'\ncluster_agent.collect_kubernetes_tags: \"true\"\ncluster_agent.kubernetes_service_name: datadog-agent-linux-cluster-agent\ncluster_agent.language_detection.patcher.enabled: \"false\"\ncluster_agent.service_account_name: datadog-agent-linux-cluster-agent\ncluster_agent.token_name: datadog-agent-linuxtoken\ncluster_checks.enabled: \"true\"\ncluster_name: my-cluster\ncollect_kubernetes_events: \"true\"\nextra_config_providers: kube_endpoints kube_services\nextra_listeners: kube_endpoints kube_services\nhealth_port: \"5556\"\ninternal_profiling.enabled: \"true\"\nkube_resources_namespace: datadog-agent-helm\nkubernetes_events_source_detection.enabled: \"false\"\nkubernetes_namespace_labels_as_tags: '{\"kubernetes.io/metadata.name\":\"name\"}'\nkubernetes_use_endpoint_slices: \"false\"\nlanguage_detection.enabled: \"false\"\nlanguage_detection.reporting.enabled: \"false\"\nleader_election: \"true\"\nleader_election_default_resource: configmap\nleader_lease_duration: \"15\"\nleader_lease_name: datadog-agent-linux-leader-election\nlog_level: INFO\norchestrator_explorer.container_scrubbing.enabled: \"true\"\norchestrator_explorer.enabled: \"true\"\nproxy:\n  http: \"\"\n  https: \"\"\n  no_proxy:\n  - 169.254.169.254\n  - 100.100.100.200\nremote_configuration.enabled: \"false\"\nsecret_backend_command_allow_group_exec_perm: \"true\"\nsecurity_agent.internal_profiling.api_key: '***************************aaaaa'\nsslkeylogfile: /tmp/sslkeylog.txt\n",
        "feature_admission_controller_auto_instrumentation_enabled": true,
        "feature_admission_controller_cws_instrumentation_enabled": false,
        "feature_admission_controller_enabled": true,
        "feature_admission_controller_inject_config_enabled": true,
        "feature_admission_controller_inject_tags_enabled": true,
        "feature_admission_controller_mutation_enabled": true,
        "feature_admission_controller_validation_enabled": true,
        "feature_apm_config_instrumentation_enabled": false,
        "feature_autoscaling_cluster_enabled": false,
        "feature_autoscaling_workload_enabled": false,
        "feature_cluster_checks_advanced_dispatching_enabled": true,
        "feature_cluster_checks_enabled": true,
        "feature_cluster_checks_exclude_checks": ["disk"],
        "feature_compliance_config_enabled": false,
        "feature_external_metrics_provider_enabled": false,
        "feature_external_metrics_provider_use_datadogmetric_crd": false,
        "feature_remote_configuration_enabled": false,
        "file_configuration": "{}\n",
        "flavor": "cluster_agent",
        "fleet_policies_configuration": "{}\n",
        "full_configuration": "ac_exclude: []\nac_include: []\nadmission_controller:\n  enabled: true\n  failure_policy: Ignore\n  mutate_unlabelled: false\n  port: 8000\napi_key: '***************************aaaaa'\ncluster_checks:\n  enabled: true\n  exclude_checks:\n  - disk\ncluster_name: my-cluster\nleader_election: true\nlog_level: INFO\n",
        "install_method_installer_version": "datadog-3.110.2",
        "install_method_tool": "helm",
        "install_method_tool_version": "Helm",
        "is_leader": true,
        "leader_election": true,
        "pod_name": "datadog-agent-linux-cluster-agent-6d8f7b9c5-x2k4p",
        "provided_configuration": "admission_controller:\n  enabled: true\napi_key: '***************************aaaaa'\ncluster_checks:\n  enabled: true\n  exclude_checks:\n  - disk\ncluster_name: my-cluster\nleader_election: true\n",
        "remote_configuration": "{}\n",
        "source_local_configuration": "{}\n"
    },
    "uuid": "00000000-0000-0000-0000-000000000001"
}
```
