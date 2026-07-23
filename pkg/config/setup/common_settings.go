// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package setup defines the configuration of the agent
package setup

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func initCoreAgentFull(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("auto_exit.validation_period", 60)
	config.BindEnvAndSetDefault("auto_exit.noprocess.enabled", false)
	config.BindEnvAndSetDefault("auto_exit.noprocess.excluded_processes", []string{})

	// The number of commits before expiring a context. The value is 2 to handle
	// the case where a check miss to send a metric.
	config.BindEnvAndSetDefault("check_sampler_bucket_commits_count_expiry", 2)
	// The number of seconds before removing stateful metric data after expiring a
	// context. Default is 25h, to minimise problems for checks that emit metircs
	// only occasionally.
	config.BindEnvAndSetDefault("check_sampler_stateful_metric_expiration_time", 25*time.Hour)
	config.BindEnvAndSetDefault("check_sampler_expire_metrics", true)
	config.BindEnvAndSetDefault("check_sampler_context_metrics", false)
	config.BindEnvAndSetDefault("check_sampler_allow_sketch_bucket_reset", true)

	config.BindEnvAndSetDefault("metric_lookback.enabled", false)
	config.BindEnvAndSetDefault("metric_lookback.enabled_checks", []string{})
	config.BindEnvAndSetDefault("metric_lookback.collection_interval", time.Second)
	config.BindEnvAndSetDefault("metric_lookback.capacity", 262144)
	config.BindEnvAndSetDefault("metric_lookback.shard_count", 16)
	config.BindEnvAndSetDefault("metric_lookback.dogstatsd.metric_names", []string{})
	config.BindEnvAndSetDefault("metric_lookback.monitor.mode", "disabled")
	config.BindEnvAndSetDefault("metric_lookback.monitor.metric_name", "")
	config.BindEnvAndSetDefault("metric_lookback.monitor.evaluation_interval", 30*time.Second)
	config.BindEnvAndSetDefault("metric_lookback.monitor.range_epsilon", float64(0))
	config.BindEnvAndSetDefault("metric_lookback.monitor.partition_tags", []string{})
	config.BindEnvAndSetDefault("metric_lookback.egress.pre_trigger_window", 0*time.Second)
	config.BindEnvAndSetDefault("metric_lookback.egress.post_recovery_window", 30*time.Second)

	config.BindEnvAndSetDefault("host_aliases", []string{})
	config.BindEnvAndSetDefault("collect_ccrid", true)

	// overridden in IoT Agent main
	config.BindEnvAndSetDefault("iot_host", false)
	// overridden in Heroku buildpack
	config.BindEnvAndSetDefault("heroku_dyno", false)

	// Python 3 linter timeout, in seconds
	// NOTE: linter is notoriously slow, in the absence of a better solution we
	// can only increase this timeout value. Linting operation is async.
	config.BindEnvAndSetDefault("python3_linter_timeout", 120)

	// Whether to honour the value of PYTHONPATH, if set, on Windows. On other OSes we always do.
	config.BindEnvAndSetDefault("windows_use_pythonpath", false)

	// When the Python full interpreter path cannot be deduced via heuristics, the agent
	// is expected to prevent rtloader from initializing. When set to true, this override
	// allows us to proceed but with some capabilities unavailable (e.g. `multiprocessing`
	// library support will not work reliably in those environments)
	config.BindEnvAndSetDefault("allow_python_path_heuristics_failure", false)

	// If true, Python is loaded when the first Python check is loaded.
	// Otherwise, Python is loaded when the collector is initialized.
	config.BindEnvAndSetDefault("python_lazy_loading", true)

	// If true, then the go loader will be prioritized over the python loader.
	config.BindEnvAndSetDefault("prioritize_go_check_loader", true)

	// If true, then new version of disk v2 check will be used.
	// Otherwise, the python version of disk check will be used.
	config.BindEnvAndSetDefault("use_diskv2_check", true)
	config.BindEnvAndSetDefault("disk_check.use_core_loader", true)

	// the darwin and bsd network check has not been ported from python
	// If true, then new version of network v2 check will be used.
	// Otherwise, the python version of network check will be used.
	config.BindEnvAndSetDefault("use_networkv2_check", GetPlatformDefault(map[string]interface{}{
		"linux":   true,
		"windows": true,
		"other":   false,
	}))
	config.BindEnvAndSetDefault("network_check.use_core_loader", GetPlatformDefault(map[string]interface{}{
		"linux":   true,
		"windows": true,
		"other":   false,
	}))

	// if/when the default is changed to true, make the default platform
	// dependent; default should remain false on Windows to maintain backward
	// compatibility with Agent5 behavior/win
	config.BindEnvAndSetDefault("hostname_fqdn", false)

	// When enabled, hostname defined in the configuration (datadog.yaml) and starting with `ip-` or `domu` on EC2 is used as
	// canonical hostname, otherwise the instance-id is used as canonical hostname.
	config.BindEnvAndSetDefault("hostname_force_config_as_canonical", false)

	// By default the Agent does not trust the hostname value retrieved from non-root UTS namespace.
	// When enabled, the Agent will trust the value retrieved from non-root UTS namespace instead of failing
	// hostname resolution.
	// (Linux only)
	config.BindEnvAndSetDefault("hostname_trust_uts_namespace", false)

	// Internal hostname drift detection configuration
	// These options are not exposed to customers and are used for testing purposes
	config.BindEnvAndSetDefault("hostname_drift_initial_delay", 20*time.Minute)
	config.BindEnvAndSetDefault("hostname_drift_recurring_interval", 6*time.Hour)

	config.BindEnvAndSetDefault("cluster_name", "")
	config.BindEnvAndSetDefault("disable_cluster_name_tag_key", false)
	config.BindEnvAndSetDefault("enabled_rfc1123_compliant_cluster_name_tag", true)

	config.BindEnvAndSetDefault("secret_backend_type", "")
	config.BindEnvAndSetDefault("secret_backend_config", map[string]interface{}{})
	config.BindEnvAndSetDefault("multi_secret_backends", map[string]interface{}{})
	config.BindEnvAndSetDefault("secret_backend_command", "")
	config.BindEnvAndSetDefault("secret_backend_arguments", []string{})
	config.BindEnvAndSetDefault("secret_backend_output_max_size", 1024*1024)
	config.BindEnvAndSetDefault("secret_backend_timeout", 30)
	config.BindEnvAndSetDefault("secret_backend_command_allow_group_exec_perm", false)
	config.BindEnvAndSetDefault("secret_backend_skip_checks", false)
	config.BindEnvAndSetDefault("secret_backend_remove_trailing_line_break", false)
	config.BindEnvAndSetDefault("secret_refresh_interval", 0)
	config.BindEnvAndSetDefault("secret_refresh_on_api_key_failure_interval", 0)
	config.BindEnvAndSetDefault("secret_refresh_scatter", true)
	config.BindEnvAndSetDefault("secret_scope_integration_to_their_k8s_namespace", false)
	config.BindEnvAndSetDefault("secret_allowed_k8s_namespace", []string{})
	config.BindEnvAndSetDefault("secret_image_to_handle", map[string][]string{})
	config.BindEnvAndSetDefault("secret_audit_file_max_size", 1024*1024)

	// IPC API server timeout
	config.BindEnvAndSetDefault("server_timeout", 30)

	// Defaults to safe YAML methods in base and custom checks.
	config.BindEnvAndSetDefault("disable_unsafe_yaml", true)

	config.BindEnvAndSetDefault("flare_provider_timeout", 10*time.Second)
	config.BindEnvAndSetDefault("flare.rc_profiling.profile_duration", 30*time.Second)
	config.BindEnvAndSetDefault("flare.profile_overhead_runtime", 10*time.Second)
	config.BindEnvAndSetDefault("flare.rc_profiling.blocking_rate", 0)
	config.BindEnvAndSetDefault("flare.rc_profiling.mutex_fraction", 0)
	config.BindEnvAndSetDefault("flare.rc_streamlogs.duration", 60*time.Second)

	config.BindEnvAndSetDefault("docker_query_timeout", int64(5))
	config.BindEnvAndSetDefault("kubernetes_node_annotations_as_host_aliases", []string{"cluster.k8s.io/machine"})
	config.BindEnvAndSetDefault("kubernetes_node_label_as_cluster_name", "")

	// Enables the prometheus config provider
	config.BindEnvAndSetDefault("prometheus_scrape.enabled", false)
	// Enables Service Endpoints checks in the prometheus config provider
	config.BindEnvAndSetDefault("prometheus_scrape.service_endpoints", false)
	// Defines any extra prometheus/openmetrics check configurations to be handled by the prometheus config provider
	config.BindEnvAndSetDefault("prometheus_scrape.checks", "")
	// Version of the openmetrics check to be scheduled by the Prometheus auto-discovery
	config.BindEnvAndSetDefault("prometheus_scrape.version", 1)
	// List of HTTP SD endpoints to poll. Each entry must specify url and check_template.
	// DD_PROMETHEUS_HTTP_SD_CONFIGS is a JSON-encoded list-of-objects.
	config.BindEnvAndSetDefault("prometheus_http_sd.configs", []map[string]interface{}{})
	config.ParseEnvJSON("prometheus_http_sd.configs", []map[string]interface{}{})
	// [DEPRECATED] URL of the Prometheus HTTP SD endpoint. Use prometheus_http_sd.configs instead.
	config.BindEnvAndSetDefault("prometheus_http_sd.url", "")
	// [DEPRECATED] JSON check template applied to each discovered target. Use prometheus_http_sd.configs instead.
	config.BindEnvAndSetDefault("prometheus_http_sd.check_template", "")

	bindEnvAndSetLogsConfigKeys(config, "network_devices.metadata.")
	config.BindEnvAndSetDefault("network_devices.namespace", "default")

	config.BindEnvAndSetDefault("snmp_listener.discovery_interval", 3600)
	config.BindEnvAndSetDefault("snmp_listener.allowed_failures", 3)
	config.BindEnvAndSetDefault("snmp_listener.discovery_allowed_failures", 3)
	config.BindEnvAndSetDefault("snmp_listener.collect_device_metadata", true)
	config.BindEnvAndSetDefault("snmp_listener.collect_topology", true)
	config.BindEnvAndSetDefault("snmp_listener.workers", 2)
	config.BindEnvAndSetDefault("snmp_listener.configs", []map[string]interface{}{})
	config.BindEnvAndSetDefault("snmp_listener.loader", "core")
	config.BindEnvAndSetDefault("snmp_listener.min_collection_interval", 15)
	config.BindEnvAndSetDefault("snmp_listener.namespace", "default")
	config.BindEnvAndSetDefault("snmp_listener.use_device_id_as_hostname", false)
	config.BindEnvAndSetDefault("snmp_listener.ping.enabled", false)
	config.BindEnvAndSetDefault("snmp_listener.ping.count", 2)
	config.BindEnvAndSetDefault("snmp_listener.ping.interval", 10)
	config.BindEnvAndSetDefault("snmp_listener.ping.timeout", 3000)
	config.BindEnvAndSetDefault("snmp_listener.ping.linux.use_raw_socket", false)
	config.BindEnvAndSetDefault("snmp_listener.oid_batch_size", 5)
	config.BindEnvAndSetDefault("snmp_listener.timeout", 5)
	config.BindEnvAndSetDefault("snmp_listener.retries", 3)

	// network_devices.autodiscovery has precedence over snmp_listener config
	// snmp_listener config is still here for legacy reasons
	config.BindEnvAndSetDefault("network_devices.autodiscovery.discovery_interval", 3600)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.allowed_failures", 3)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.discovery_allowed_failures", 3)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.collect_device_metadata", true)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.collect_topology", true)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.workers", 2)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.configs", []map[string]interface{}{})
	config.BindEnvAndSetDefault("network_devices.autodiscovery.loader", "core")
	config.BindEnvAndSetDefault("network_devices.autodiscovery.min_collection_interval", 15)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.namespace", "default")
	config.BindEnvAndSetDefault("network_devices.autodiscovery.use_device_id_as_hostname", false)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.ping.enabled", false)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.ping.count", 2)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.ping.interval", 10)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.ping.timeout", 3000)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.ping.linux.use_raw_socket", false)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.use_deduplication", false)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.collect_vpn", false)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.oid_batch_size", 5)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.timeout", 5)
	config.BindEnvAndSetDefault("network_devices.autodiscovery.retries", 3)

	config.BindEnvAndSetDefault("network_devices.default_scan.enabled", true)
	config.BindEnvAndSetDefault("network_devices.default_scan.excluded_ips", []string{})

	bindEnvAndSetLogsConfigKeys(config, "network_devices.snmp_traps.forwarder.")
	config.BindEnvAndSetDefault("network_devices.snmp_traps.enabled", false)
	config.BindEnvAndSetDefault("network_devices.snmp_traps.port", 9162)
	config.BindEnvAndSetDefault("network_devices.snmp_traps.community_strings", []string{})
	config.BindEnvAndSetDefault("network_devices.snmp_traps.bind_host", "0.0.0.0")
	// in seconds
	config.BindEnvAndSetDefault("network_devices.snmp_traps.stop_timeout", 5)
	config.BindEnvAndSetDefault("network_devices.snmp_traps.users", []map[string]string{})
	config.BindEnvAndSetDefault("network_devices.snmp_traps.tags", []string{})

	config.BindEnvAndSetDefault("network_devices.netflow.enabled", false)
	config.SetDefault("network_devices.netflow.listeners", []map[string]interface{}{})
	config.BindEnvAndSetDefault("network_devices.netflow.stop_timeout", 5)
	config.BindEnvAndSetDefault("network_devices.netflow.aggregator_buffer_size", 10000)
	config.BindEnvAndSetDefault("network_devices.netflow.aggregator_flush_interval", 300)
	// The default behavior for this value is to copy the "aggregator_flush_interval" when absent/zero. This is to avoid
	// expiring flows too early when the flush interval is increased, while still allowing to set a custom TTL when needed.
	config.BindEnvAndSetDefault("network_devices.netflow.aggregator_flow_context_ttl", 0)
	config.BindEnvAndSetDefault("network_devices.netflow.aggregator_port_rollup_threshold", 10)
	config.BindEnvAndSetDefault("network_devices.netflow.aggregator_rollup_tracker_refresh_interval", 300)
	bindEnvAndSetLogsConfigKeys(config, "network_devices.netflow.forwarder.")
	config.BindEnvAndSetDefault("network_devices.netflow.reverse_dns_enrichment_enabled", false)

	config.BindEnvAndSetDefault("network_path.connections_monitoring.enabled", false)
	config.BindEnvAndSetDefault("network_path.netflow_monitoring.enabled", false)
	config.BindEnvAndSetDefault("network_path.remote_config.enabled", false)
	config.BindEnvAndSetDefault("network_path.collector.workers", 4)
	config.BindEnvAndSetDefault("network_path.collector.timeout", DefaultNetworkPathTimeout)
	config.BindEnvAndSetDefault("network_path.collector.max_ttl", DefaultNetworkPathMaxTTL)
	config.BindEnvAndSetDefault("network_path.collector.input_chan_size", 1000)
	config.BindEnvAndSetDefault("network_path.collector.processing_chan_size", 1000)
	config.BindEnvAndSetDefault("network_path.collector.pathtest_contexts_limit", 1000)
	// with 30min interval, 70m will allow running a test 3 times (t0, t30, t60)
	config.BindEnvAndSetDefault("network_path.collector.pathtest_ttl", "70m")
	config.BindEnvAndSetDefault("network_path.collector.pathtest_interval", "30m")
	config.BindEnvAndSetDefault("network_path.collector.flush_interval", "10s")
	config.BindEnvAndSetDefault("network_path.collector.pathtest_max_per_minute", 150)
	config.BindEnvAndSetDefault("network_path.collector.pathtest_max_burst_duration", "30s")
	config.BindEnvAndSetDefault("network_path.collector.reverse_dns_enrichment.enabled", true)
	config.BindEnvAndSetDefault("network_path.collector.reverse_dns_enrichment.timeout", 5000)
	config.BindEnvAndSetDefault("network_path.collector.disable_intra_vpc_collection", false)
	config.BindEnvAndSetDefault("network_path.collector.disable_source_public_ip_collection", false)
	config.BindEnvAndSetDefault("network_path.collector.source_excludes", map[string][]string{})
	config.BindEnvAndSetDefault("network_path.collector.dest_excludes", map[string][]string{})
	config.BindEnvAndSetDefault("network_path.collector.tcp_method", "")
	config.BindEnvAndSetDefault("network_path.collector.icmp_mode", "")
	config.BindEnvAndSetDefault("network_path.collector.tcp_syn_paris_traceroute_mode", false)
	config.BindEnvAndSetDefault("network_path.collector.traceroute_queries", DefaultNetworkPathStaticPathTracerouteQueries)
	config.BindEnvAndSetDefault("network_path.collector.e2e_queries", DefaultNetworkPathStaticPathE2eQueries)
	config.BindEnvAndSetDefault("network_path.collector.disable_windows_driver", false)
	config.BindEnvAndSetDefault("network_path.collector.monitor_ip_without_domain", false)
	config.BindEnvAndSetDefault("network_path.collector.filters", []map[string]string{})

	bindEnvAndSetLogsConfigKeys(config, "network_path.forwarder.")

	bindEnvAndSetLogsConfigKeys(config, "network_devices.config_management.forwarder.")
	config.BindEnvAndSetDefault("network_devices.config_management.rollback.enabled", false)

	config.BindEnvAndSetDefault("ha_agent.enabled", false)
	config.BindEnvAndSetDefault("ha_agent.group", "")

	config.BindEnvAndSetDefault("kubernetes_kubeconfig_path", "")
	config.BindEnvAndSetDefault("kubernetes_apiserver_ca_path", "")
	config.BindEnvAndSetDefault("kubernetes_apiserver_tls_verify", true)
	config.BindEnvAndSetDefault("leader_lease_duration", "60")
	config.BindEnvAndSetDefault("leader_election", false)
	config.BindEnvAndSetDefault("leader_lease_name", "datadog-leader-election")
	config.BindEnvAndSetDefault("leader_election_default_resource", "configmap")
	config.BindEnvAndSetDefault("leader_election_release_on_shutdown", true)
	config.BindEnvAndSetDefault("kube_resources_namespace", "")
	config.BindEnvAndSetDefault("kube_cache_sync_timeout_seconds", 10)

	config.BindEnvAndSetDefault("cluster_agent.enabled", false)
	config.BindEnvAndSetDefault("cluster_agent.cmd_port", 5005)
	config.BindEnvAndSetDefault("cluster_agent.allow_legacy_tls", false)
	config.BindEnvAndSetDefault("cluster_agent.auth_token", "")
	config.BindEnvAndSetDefault("cluster_agent.url", "")
	config.BindEnvAndSetDefault("cluster_agent.kubernetes_service_name", "datadog-cluster-agent")
	config.BindEnvAndSetDefault("cluster_agent.service_account_name", "")
	config.BindEnvAndSetDefault("cluster_agent.tagging_fallback", false)
	config.BindEnvAndSetDefault("cluster_agent.server.read_timeout_seconds", 2)
	config.BindEnvAndSetDefault("cluster_agent.server.write_timeout_seconds", 2)
	config.BindEnvAndSetDefault("cluster_agent.server.idle_timeout_seconds", 60)
	config.BindEnvAndSetDefault("cluster_agent.refresh_on_cache_miss", true)
	config.BindEnvAndSetDefault("cluster_agent.serve_nozzle_data", false)
	config.BindEnvAndSetDefault("cluster_agent.sidecars_tags", false)
	config.BindEnvAndSetDefault("cluster_agent.isolation_segments_tags", false)
	config.BindEnvAndSetDefault("cluster_agent.token_name", "datadogtoken")
	config.BindEnvAndSetDefault("cluster_agent.max_leader_connections", 100)
	config.BindEnvAndSetDefault("cluster_agent.client_reconnect_period_seconds", 1200)
	config.BindEnvAndSetDefault("cluster_agent.collect_kubernetes_tags", false)
	config.BindEnvAndSetDefault("cluster_agent.kubernetes_resources_collection.pod_annotations_exclude", []string{
		`^kubectl\.kubernetes\.io\/last-applied-configuration$`,
		`^ad\.datadoghq\.com\/([[:alnum:]]+\.)?(checks|check_names|init_configs|instances)$`,
	})
	config.BindEnvAndSetDefault("cluster_agent.kubernetes_resources_collection.deployment_annotations_exclude", []string{
		`^kubectl\.kubernetes\.io\/last-applied-configuration$`,
		`^ad\.datadoghq\.com\/([[:alnum:]]+\.)?(checks|check_names|init_configs|instances)$`,
	})
	config.BindEnvAndSetDefault("metrics_port", 5000)
	config.BindEnvAndSetDefault("cluster_agent.language_detection.patcher.enabled", true)
	config.BindEnvAndSetDefault("cluster_agent.language_detection.patcher.base_backoff", "5m")
	config.BindEnvAndSetDefault("cluster_agent.language_detection.patcher.max_backoff", "1h")
	// sets the expiration deadline (TTL) for reported languages
	config.BindEnvAndSetDefault("cluster_agent.language_detection.cleanup.language_ttl", "30m")
	// language annotation cleanup period
	config.BindEnvAndSetDefault("cluster_agent.language_detection.cleanup.period", "10m")

	// AppSec Injector in the cluster agent ( Preview )
	config.BindEnvAndSetDefault("cluster_agent.appsec.injector.enabled", false)
	config.BindEnvAndSetDefault("cluster_agent.appsec.injector.base_backoff", "5m")
	config.BindEnvAndSetDefault("cluster_agent.appsec.injector.max_backoff", "1h")
	config.BindEnvAndSetDefault("cluster_agent.appsec.injector.labels", map[string]string{})
	config.BindEnvAndSetDefault("cluster_agent.appsec.injector.annotations", map[string]string{})
	config.BindEnvAndSetDefault("cluster_agent.appsec.injector.processor.service.name", "")
	config.BindEnvAndSetDefault("cluster_agent.appsec.injector.processor.service.namespace", "")
	config.BindEnvAndSetDefault("cluster_agent.appsec.injector.istio.namespace", "istio-system")
	config.BindEnvAndSetDefault("cluster_agent.appsec.injector.envoy_gateway.namespace", "envoy-gateway-system")
	config.BindEnvAndSetDefault("cluster_agent.appsec.injector.envoy_gateway.controller_namespace", "envoy-gateway-system")
	config.BindEnvAndSetDefault("cluster_agent.appsec.injector.mode", "sidecar")

	// APM tracing for the cluster agent itself (currently covers cluster check dispatching)
	config.BindEnvAndSetDefault("cluster_agent.tracing.enabled", false)
	config.BindEnvAndSetDefault("cluster_agent.tracing.env", "")
	config.BindEnvAndSetDefault("cluster_agent.tracing.sample_rate", float64(0.1))

	config.BindEnvAndSetDefault("admission_controller.appsec.sidecar.image", "ghcr.io/datadog/dd-trace-go/service-extensions-callout")
	config.BindEnvAndSetDefault("admission_controller.appsec.sidecar.image_tag", "latest")
	config.BindEnvAndSetDefault("admission_controller.appsec.sidecar.port", 8080)
	config.BindEnvAndSetDefault("admission_controller.appsec.sidecar.health_port", 8081)
	config.BindEnvAndSetDefault("admission_controller.appsec.sidecar.resources.requests.cpu", "10m")
	config.BindEnvAndSetDefault("admission_controller.appsec.sidecar.resources.requests.memory", "128Mi")
	config.BindEnvAndSetDefault("admission_controller.appsec.sidecar.resources.limits.cpu", "")
	config.BindEnvAndSetDefault("admission_controller.appsec.sidecar.resources.limits.memory", "")
	config.BindEnvAndSetDefault("admission_controller.appsec.sidecar.body_parsing_size_limit", "")
	config.BindEnvAndSetDefault("admission_controller.appsec.sidecar.uds_path", "/var/run/datadog/extproc.sock")
	config.BindEnvAndSetDefault("admission_controller.appsec.sidecar.run_as_user", 65532)

	config.BindEnvAndSetDefault("admission_controller.appsec.nginx.init_image", "datadog/ingress-nginx-injection")
	config.BindEnvAndSetDefault("admission_controller.appsec.nginx.module_mount_path", "/modules_mount")
	// Non-root UID/GID for the injected init container. Defaults match the
	// stock datadog/ingress-nginx-injection image, which declares no USER and
	// would otherwise be rejected under runAsNonRoot. Set to a negative value
	// to leave the security context unset and honor a custom init_image's own USER.
	config.BindEnvAndSetDefault("admission_controller.appsec.nginx.init_run_as_user", 101)
	config.BindEnvAndSetDefault("admission_controller.appsec.nginx.init_run_as_group", 82)

	config.BindEnvAndSetDefault("cluster_agent.kube_metadata_collection.enabled", false)
	config.BindEnvAndSetDefault("cluster_agent.kueue.enabled", false)
	// list of kubernetes resources for which we collect metadata
	// each resource is specified in the format `{group}/{version}/{resource}` or `{group}/{resource}`
	// resources that belong to the empty group can be specified simply as `{resource}` or as `/{resource}`
	//
	// the version is optional and can be left empty, and in this case, the agent will automatically assign
	// the version that is considered by the api server as the preferred version for the related group.
	//
	// examples with version:
	// - apps/v1/deployments
	// - /v1/nodes
	//
	// examples without version:
	// - apps/deployments
	// - /nodes
	// - nodes
	config.BindEnvAndSetDefault("cluster_agent.kube_metadata_collection.resources", []string{})
	config.BindEnvAndSetDefault("cluster_agent.kube_metadata_collection.resource_annotations_exclude", []string{})
	// partially deprecated: `agent_ipc.grpc_max_message_size` should now be used for configuring the maximum gRPC
	// message size on the agent IPC server
	//
	// this is still used directly for determining the chunking behavior of messages sent from the remote tagger and
	// remote workloadmeta, to preserve existing behavior until we can better unify chunking behavior across all the
	// gRPC services on the IPC endpoint.
	//
	// if this value is larger than `agent_ipc.grpc_max_message_size`, it will still be honoured by the agent gRPC
	// server instead of `agent_ipc.grpc_max_message_size`.
	config.BindEnvAndSetDefault("cluster_agent.cluster_tagger.grpc_max_message_size", 4194304)

	// Check that the trust chain is valid for Agent cross-node communications (NodeAgent->DCA / CLC->DCA / DCA->CLC).
	config.BindEnvAndSetDefault("cluster_trust_chain.enable_tls_verification", false)
	// Path to the cluster CA certificate file.
	config.BindEnvAndSetDefault("cluster_trust_chain.ca_cert_file_path", "")
	// Path to the cluster CA key file.
	config.BindEnvAndSetDefault("cluster_trust_chain.ca_key_file_path", "")

	// the entity id, typically set by dca admisson controller config mutator, used for external origin detection
	config.SetDefault("entity_id", "")

	// Defines the maximum size of hostame gathered from EC2, GCE, Azure, Alibaba, Oracle and Tencent cloud metadata
	// endpoints (all cloudprovider except IBM). IBM cloud ignore this setting as their API return a huge JSON with
	// all the metadata for the VM.
	// Used internally to protect against configurations where metadata endpoints return incorrect values with 200 status codes.
	config.BindEnvAndSetDefault("metadata_endpoints_max_hostname_size", 255)

	config.BindEnvAndSetDefault("ec2_use_windows_prefix_detection", false)
	// value in milliseconds
	config.BindEnvAndSetDefault("ec2_metadata_timeout", 300)
	// value in seconds
	config.BindEnvAndSetDefault("ec2_metadata_token_lifetime", 21600)
	config.BindEnvAndSetDefault("ec2_prefer_imdsv2", false)
	// used to bypass the hostname detection logic and force the EC2 instance ID as a hostname.
	config.BindEnvAndSetDefault("ec2_prioritize_instance_id_as_hostname", false)
	// should the agent leverage DMI information to know if it's running on EC2 or not. Enabling this will add the instance ID from DMI to the host alias list.
	config.BindEnvAndSetDefault("ec2_use_dmi", true)
	config.BindEnvAndSetDefault("collect_ec2_tags", false)
	config.BindEnvAndSetDefault("collect_ec2_instance_info", false)
	config.BindEnvAndSetDefault("collect_ec2_tags_use_imds", false)
	config.BindEnvAndSetDefault("exclude_ec2_tags", []string{})
	config.BindEnvAndSetDefault("ec2_imdsv2_transition_payload_enabled", true)

	// Will be autodetected
	config.BindEnvAndSetDefault("ecs_agent_url", "")
	config.BindEnvAndSetDefault("ecs_agent_container_name", "ecs-agent")
	config.BindEnvAndSetDefault("ecs_resource_tags_replace_colon", false)
	// value in milliseconds
	config.BindEnvAndSetDefault("ecs_metadata_timeout", 1000)
	config.BindEnvAndSetDefault("ecs_metadata_retry_initial_interval", 100*time.Millisecond)
	config.BindEnvAndSetDefault("ecs_metadata_retry_max_elapsed_time", 3000*time.Millisecond)
	config.BindEnvAndSetDefault("ecs_metadata_retry_timeout_factor", 3)
	config.BindEnvAndSetDefault("ecs_task_collection_enabled", true)
	config.BindEnvAndSetDefault("ecs_task_cache_ttl", 3*time.Minute)
	config.BindEnvAndSetDefault("ecs_task_collection_rate", 35)
	config.BindEnvAndSetDefault("ecs_task_collection_burst", 60)
	config.BindEnvAndSetDefault("ecs_deployment_mode", "auto")

	config.BindEnvAndSetDefault("collect_gce_tags", true)
	config.BindEnvAndSetDefault("exclude_gce_tags", []string{
		"bosh_settings", "cli-cert", "common-psm1", "configure-sh", "containerd-configure-sh",
		"disable-address-manager", "disable-legacy-endpoints", "enable-oslogin", "gce-container-declaration",
		"google-container-manifest", "ipsec-cert", "k8s-node-setup-psm1", "kube-env", "kubeconfig",
		"kubelet-config", "serial-port-logging-enable", "shutdown-script", "ssh-keys", "sshKeys", "ssl-cert",
		"startup-script", "user-data", "windows-keys", "windows-startup-script-ps1",
	})
	config.BindEnvAndSetDefault("gce_send_project_id_tag", false)
	// value in milliseconds
	config.BindEnvAndSetDefault("gce_metadata_timeout", 1000)

	config.BindEnvAndSetDefault("collect_gpu_tags", true)
	config.BindEnvAndSetDefault("gpu.enabled", false)
	config.BindEnvAndSetDefault("gpu.nvml_lib_path", "")
	config.BindEnvAndSetDefault("gpu.use_sp_process_metrics", false)
	config.BindEnvAndSetDefault("gpu.sp_process_metrics_request_timeout", 3*time.Second)
	config.BindEnvAndSetDefault("gpu.integrate_with_workloadmeta_processes", true)
	config.BindEnvAndSetDefault("gpu.workload_tag_cache_size", 1024)
	config.BindEnvAndSetDefault("gpu.disabled_collectors", []string{})
	config.BindEnvAndSetDefault("gpu.nvlink.fec_light_error_threshold", 3)
	config.BindEnvAndSetDefault("gpu.parallel_collectors", true)
	// gpu.collection_interval_override (seconds) overrides the gpu check scheduling
	// cadence when > 0, taking precedence over the instance's min_collection_interval.
	// Binds DD_GPU_COLLECTION_INTERVAL_OVERRIDE.
	config.BindEnvAndSetDefault("gpu.collection_interval_override", 0)

	config.BindEnvAndSetDefault("gpu.nccl.enabled", false)
	config.BindEnvAndSetDefault("gpu.nccl.socket_path", "/var/run/datadog/nccl.socket")
	// host_socket_path is read by the helm chart and operator to decide whether
	// to mount /var/run/datadog into the agent pod. For DSD/APM-enabled deployments
	// the directory is already mounted; this setting matters for NCCL-only setups.
	config.BindEnvAndSetDefault("gpu.nccl.host_socket_path", "/var/run/datadog")
	// In-container directory at which the agent's NCCL socket is mounted in
	// injected workload pods. Composed with filepath.Base(gpu.nccl.socket_path)
	// to build the mount destination and NCCL_DD_SOCKET_PATH. A dedicated sibling
	// of /var/run/datadog (not that dir itself like APM/DSD, and not a subdir of
	// it -- a child mount nested inside the APM/DSD mount is unsupported) so this
	// directory mount never collides with the APM/DSD config webhook's mount.
	config.BindEnvAndSetDefault("admission_controller.nccl_profiler.socket_dir", "/var/run/datadog-nccl")

	config.BindEnvAndSetDefault("cloud_foundry_bbs.url", "https://bbs.service.cf.internal:8889")
	config.BindEnvAndSetDefault("cloud_foundry_bbs.poll_interval", 15)
	config.BindEnvAndSetDefault("cloud_foundry_bbs.ca_file", "")
	config.BindEnvAndSetDefault("cloud_foundry_bbs.cert_file", "")
	config.BindEnvAndSetDefault("cloud_foundry_bbs.key_file", "")
	config.BindEnvAndSetDefault("cloud_foundry_bbs.env_include", []string{})
	config.BindEnvAndSetDefault("cloud_foundry_bbs.env_exclude", []string{})
	config.BindEnvAndSetDefault("cloud_foundry_cc.url", "https://cloud-controller-ng.service.cf.internal:9024")
	config.BindEnvAndSetDefault("cloud_foundry_cc.client_id", "")
	config.BindEnvAndSetDefault("cloud_foundry_cc.client_secret", "")
	config.BindEnvAndSetDefault("cloud_foundry_cc.poll_interval", 60)
	config.BindEnvAndSetDefault("cloud_foundry_cc.skip_ssl_validation", false)
	config.BindEnvAndSetDefault("cloud_foundry_cc.apps_batch_size", 5000)
	config.BindEnvAndSetDefault("cloud_foundry_garden.listen_network", "unix")
	config.BindEnvAndSetDefault("cloud_foundry_garden.listen_address", "/var/vcap/data/garden/garden.sock")
	config.BindEnvAndSetDefault("cloud_foundry_container_tagger.shell_path", "/bin/sh")
	config.BindEnvAndSetDefault("cloud_foundry_container_tagger.retry_count", 10)
	config.BindEnvAndSetDefault("cloud_foundry_container_tagger.retry_interval", 10)

	config.BindEnvAndSetDefault("azure_hostname_style", "os")
	config.BindEnvAndSetDefault("azure_metadata_timeout", 300)
	config.BindEnvAndSetDefault("azure_metadata_api_version", "2021-02-01")

	// We use a long timeout here since the metadata and token API can be very slow sometimes.
	// value in seconds
	config.BindEnvAndSetDefault("ibm_metadata_timeout", 5)

	config.BindEnvAndSetDefault("jmx_custom_jars", []string{})
	config.BindEnvAndSetDefault("jmx_java_tool_options", "")
	config.BindEnvAndSetDefault("jmx_use_cgroup_memory_limit", false)
	config.BindEnvAndSetDefault("jmx_use_container_support", false)
	config.BindEnvAndSetDefault("jmx_max_ram_percentage", float64(25))
	config.BindEnvAndSetDefault("jmx_max_restarts", int64(3))
	config.BindEnvAndSetDefault("jmx_restart_interval", int64(5))
	config.BindEnvAndSetDefault("jmx_thread_pool_size", 3)
	config.BindEnvAndSetDefault("jmx_reconnection_thread_pool_size", 3)
	config.BindEnvAndSetDefault("jmx_collection_timeout", 60)
	config.BindEnvAndSetDefault("jmx_check_period", int(defaults.DefaultCheckInterval/time.Millisecond))
	config.BindEnvAndSetDefault("jmx_reconnection_timeout", 60)
	config.BindEnvAndSetDefault("jmx_statsd_telemetry_enabled", false)
	config.BindEnvAndSetDefault("jmx_telemetry_enabled", false)
	// The following jmx_statsd_client-* options are internal and will not be documented
	// the queue size is the no. of elements (metrics, event, service checks) it can hold.
	config.BindEnvAndSetDefault("jmx_statsd_client_queue_size", 4096)
	config.BindEnvAndSetDefault("jmx_statsd_client_use_non_blocking", false)
	// the "buffer" here is the socket send buffer (SO_SNDBUF) and the size is in bytes
	config.BindEnvAndSetDefault("jmx_statsd_client_buffer_size", 0)
	// the socket timeout (SO_SNDTIMEO) is in milliseconds
	config.BindEnvAndSetDefault("jmx_statsd_client_socket_timeout", 0)

	// Go_expvar server port
	config.BindEnvAndSetDefault("expvar_port", 5000)

	config.BindEnvAndSetDefault("internal_profiling.enabled", false)
	config.BindEnvAndSetDefault("internal_profiling.profile_dd_url", "")
	// file system path to a unix socket, e.g. `/var/run/datadog/apm.socket`
	config.BindEnvAndSetDefault("internal_profiling.unix_socket", "")
	config.BindEnvAndSetDefault("internal_profiling.period", 5*time.Minute)
	config.BindEnvAndSetDefault("internal_profiling.cpu_duration", 1*time.Minute)
	config.BindEnvAndSetDefault("internal_profiling.block_profile_rate", 0)
	config.BindEnvAndSetDefault("internal_profiling.mutex_profile_fraction", 0)
	config.BindEnvAndSetDefault("internal_profiling.enable_goroutine_stacktraces", false)
	config.BindEnvAndSetDefault("internal_profiling.enable_block_profiling", false)
	config.BindEnvAndSetDefault("internal_profiling.enable_mutex_profiling", false)
	config.BindEnvAndSetDefault("internal_profiling.delta_profiles", true)
	config.BindEnvAndSetDefault("internal_profiling.extra_tags", []string{})
	config.BindEnvAndSetDefault("internal_profiling.custom_attributes", []string{"check_id"})

	config.BindEnvAndSetDefault("internal_profiling.capture_all_allocations", false)

	config.BindEnvAndSetDefault("hpa_watcher_polling_freq", 10)
	// 5 minutes
	config.BindEnvAndSetDefault("hpa_watcher_gc_period", 60*5)
	config.BindEnvAndSetDefault("hpa_configmap_name", "datadog-custom-metrics")
	config.BindEnvAndSetDefault("external_metrics_provider.enabled", false)
	config.BindEnvAndSetDefault("external_metrics_provider.port", 8443)
	// Override the Datadog API endpoint to query external metrics from
	config.BindEnvAndSetDefault("external_metrics_provider.endpoint", "")
	// Override the Datadog API Key for external metrics endpoint
	config.BindEnvAndSetDefault("external_metrics_provider.api_key", "")
	// Override the Datadog APP Key for external metrics endpoint
	config.BindEnvAndSetDefault("external_metrics_provider.app_key", "")
	// List of redundant endpoints to query external metrics from
	config.SetDefault("external_metrics_provider.endpoints", []interface{}{})
	// value in seconds. Frequency of calls to Datadog to refresh metric values
	config.BindEnvAndSetDefault("external_metrics_provider.refresh_period", 30)
	// value in seconds. Batch the events from the Autoscalers informer to push updates to the ConfigMap (GlobalStore)
	config.BindEnvAndSetDefault("external_metrics_provider.batch_window", 10)
	// value in seconds. 4 cycles from the Autoscaler controller (up to Kubernetes 1.11) is enough to consider a metric stale
	config.BindEnvAndSetDefault("external_metrics_provider.max_age", 120)
	// value in seconds. Represents grace period to account for delay between metric is resolved and when autoscaling controllers query for it
	config.BindEnvAndSetDefault("external_metrics_provider.query_validity_period", 30)
	// aggregator used for the external metrics. Choose from [avg,sum,max,min]
	config.BindEnvAndSetDefault("external_metrics.aggregator", "avg")
	// Maximum window to query to get the metric from Datadog.
	config.BindEnvAndSetDefault("external_metrics_provider.max_time_window", 60*60*24)
	// Window to query to get the metric from Datadog.
	config.BindEnvAndSetDefault("external_metrics_provider.bucket_size", 60*5)
	// Bucket size to circumvent time aggregation side effects.
	config.BindEnvAndSetDefault("external_metrics_provider.rollup", 30)
	// Activates the controller for Watermark Pod Autoscalers.
	config.BindEnvAndSetDefault("external_metrics_provider.wpa_controller", false)
	// Use DatadogMetric CRD with custom Datadog Queries instead of ConfigMap
	config.BindEnvAndSetDefault("external_metrics_provider.use_datadogmetric_crd", false)
	// Enables autogeneration of DatadogMetrics when the DatadogMetric CRD is in use
	config.BindEnvAndSetDefault("external_metrics_provider.enable_datadogmetric_autogen", true)
	// Label selector to filter which HPAs and WPAs the DCA generates DatadogMetrics for (e.g. "app.kubernetes.io/managed-by!=keda-operator")
	config.BindEnvAndSetDefault("external_metrics_provider.autoscaler_autogen_label_selector", "")
	// timeout between two successful event collections in milliseconds.
	config.BindEnvAndSetDefault("kubernetes_event_collection_timeout", 100)
	// value in seconds. Default to 5 minutes
	config.BindEnvAndSetDefault("kubernetes_informers_resync_period", 60*5)
	// list of options that can be used to configure the external metrics server
	config.BindEnvAndSetDefault("external_metrics_provider.config", map[string]string{})
	// value in seconds
	config.BindEnvAndSetDefault("external_metrics_provider.local_copy_refresh_rate", 30)
	// Maximum number of queries to batch when querying Datadog.
	config.BindEnvAndSetDefault("external_metrics_provider.chunk_size", 35)
	// Splits batches and runs queries with errors individually with an exponential backoff
	config.BindEnvAndSetDefault("external_metrics_provider.split_batches_with_backoff", false)
	// Number of workers spawned by controller (only when CRD is used)
	config.BindEnvAndSetDefault("external_metrics_provider.num_workers", 2)
	// Maximum number of parallel queries sent to Datadog simultaneously
	config.BindEnvAndSetDefault("external_metrics_provider.max_parallel_queries", 10)
	config.BindEnvAndSetDefault("instrumentation_crd_controller.enabled", false)
	// TODO(CINT)(Agent 7.53+) Remove this flag when hybrid ignore_ad_tags is fully deprecated
	config.BindEnvAndSetDefault("cluster_checks.support_hybrid_ignore_ad_tags", false)
	config.BindEnvAndSetDefault("cluster_checks.enabled", false)
	// value in seconds
	config.BindEnvAndSetDefault("cluster_checks.node_expiration_timeout", 30)
	// value in seconds
	config.BindEnvAndSetDefault("cluster_checks.warmup_duration", 30)
	// value in seconds
	config.BindEnvAndSetDefault("cluster_checks.unscheduled_check_threshold", 60)
	config.BindEnvAndSetDefault("cluster_checks.cluster_tag_name", "cluster_name")
	config.BindEnvAndSetDefault("cluster_checks.extra_tags", []string{})
	config.BindEnvAndSetDefault("cluster_checks.advanced_dispatching_enabled", true)
	config.BindEnvAndSetDefault("cluster_checks.rebalance_with_utilization", true)
	// Experimental. Subject to change. Rebalance only if the distribution found improves the current one by this.
	config.BindEnvAndSetDefault("cluster_checks.rebalance_min_percentage_improvement", 10)
	config.BindEnvAndSetDefault("cluster_checks.clc_runners_port", 5005)
	config.BindEnvAndSetDefault("cluster_checks.exclude_checks", []string{})
	config.BindEnvAndSetDefault("cluster_checks.exclude_checks_from_dispatching", []string{})
	config.BindEnvAndSetDefault("cluster_checks.rebalance_period", 10*time.Minute)
	// KSM resource sharding: splits KSM check by resource type (pods, nodes, others)
	config.BindEnvAndSetDefault("cluster_checks.ksm_sharding_enabled", false)
	config.BindEnvAndSetDefault("cluster_checks.crd_collection", false)
	// Biases check placement toward the runner where a check previously ran.
	config.BindEnvAndSetDefault("cluster_checks.stickiness_enabled", true)
	// Multiplier applied to workersNeeded when computing the stickiness bias.
	config.BindEnvAndSetDefault("cluster_checks.stickiness_factor", float64(4))
	// Maximum stickiness bias applied regardless of workersNeeded.
	config.BindEnvAndSetDefault("cluster_checks.stickiness_upper_limit", float64(1))
	// Minimum stickiness bias applied when stickiness is enabled.
	config.BindEnvAndSetDefault("cluster_checks.stickiness_lower_limit", float64(0.05))

	config.BindEnvAndSetDefault("clc_runner_enabled", false)
	config.BindEnvAndSetDefault("clc_runner_id", "")
	// must be set using the Kubernetes downward API
	config.BindEnvAndSetDefault("clc_runner_host", "")
	config.BindEnvAndSetDefault("clc_runner_port", 5005)
	config.BindEnvAndSetDefault("clc_runner_server_write_timeout", 15)
	config.BindEnvAndSetDefault("clc_runner_server_readheader_timeout", 10)

	// Enabling remote tagger in cluster check runners by default allows
	// enriching the cluster check runners with tags sourced from the
	// cluster agent's cluster tagger.
	//
	// This was previously disabled because of the overhead this can
	// cause on large clusters. This is no longer an issue because
	// the tagger now supports filtering out unwanted tags and the
	// cluster check runner remote tagger filters out pod tags, making
	// the overhead relatively insignificant.
	//
	// For more details: https://github.com/DataDog/datadog-agent/blob/8af994a91cafecf647197e1638de9ddd98b06575/cmd/agent/common/tagger_params.go#L1-L39
	//
	// The benefit of activating this is allowing cluster checks and component
	// running in the cluster check runner to gain better tagging coverage
	// on emitted metrics:
	//
	// * KSM check running in CLC runner needs the remote tagger to ensure
	// namespace labels and annotations as tags are applied to emitted KSM
	// metrics in case the check is partitioned to multiple instances, each
	// emitting different resource metrics.
	config.BindEnvAndSetDefault("clc_runner_remote_tagger_enabled", true)

	config.BindEnvAndSetDefault("remote_tagger.max_concurrent_sync", 3)

	config.BindEnvAndSetDefault("csi.enabled", false)
	config.BindEnvAndSetDefault("csi.driver", "k8s.csi.datadoghq.com")

	config.BindEnvAndSetDefault("admission_controller.enabled", false)
	config.BindEnvAndSetDefault("admission_controller.validation.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.mutation.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.mutate_unlabelled", false)
	config.BindEnvAndSetDefault("admission_controller.port", 8000)
	config.BindEnvAndSetDefault("admission_controller.container_registry", "gcr.io/datadoghq")
	// in seconds (see kubernetes/kubernetes#71508)
	config.BindEnvAndSetDefault("admission_controller.timeout_seconds", 10)
	config.BindEnvAndSetDefault("admission_controller.service_name", "datadog-admission-controller")
	// validity bound of the certificate created by the controller (in hours, default 1 year)
	config.BindEnvAndSetDefault("admission_controller.certificate.validity_bound", 365*24)
	// how long before its expiration a certificate should be refreshed (in hours, default 1 month)
	config.BindEnvAndSetDefault("admission_controller.certificate.expiration_threshold", 30*24)
	// name of the Secret object containing the webhook certificate
	config.BindEnvAndSetDefault("admission_controller.certificate.secret_name", "webhook-certificate")
	config.BindEnvAndSetDefault("admission_controller.webhook_name", "datadog-webhook")
	config.BindEnvAndSetDefault("admission_controller.inject_config.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.inject_config.endpoint", "/injectconfig")
	// possible values: hostip / service / socket / csi
	config.BindEnvAndSetDefault("admission_controller.inject_config.mode", "hostip")
	config.BindEnvAndSetDefault("admission_controller.inject_config.local_service_name", "datadog")
	config.BindEnvAndSetDefault("trace_agent_host_socket_path", "/var/run/datadog")
	config.BindEnvAndSetDefault("dogstatsd_host_socket_path", "/var/run/datadog")
	config.BindEnvAndSetDefault("admission_controller.inject_config.socket_path", "/var/run/datadog")
	config.BindEnvAndSetDefault("admission_controller.inject_config.type_socket_volumes", false)
	config.BindEnvAndSetDefault("admission_controller.inject_tags.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.inject_tags.endpoint", "/injecttags")
	// in minutes
	config.BindEnvAndSetDefault("admission_controller.inject_tags.pod_owners_cache_validity", 10)
	// deprecated alias for `admission_controller.inject_tags.pod_owners_cache_validity`
	config.BindEnvAndSetDefault("admission_controller.pod_owners_cache_validity", 10)
	config.BindEnvAndSetDefault("admission_controller.namespace_selector_fallback", false)
	config.BindEnvAndSetDefault("admission_controller.failure_policy", "Ignore")
	config.BindEnvAndSetDefault("admission_controller.reinvocation_policy", "IfNeeded")
	// adds in the webhook some selectors that are required in AKS
	config.BindEnvAndSetDefault("admission_controller.add_aks_selectors", false)
	config.BindEnvAndSetDefault("admission_controller.probe.enabled", false)
	// in seconds
	config.BindEnvAndSetDefault("admission_controller.probe.interval", 60)
	// in seconds
	config.BindEnvAndSetDefault("admission_controller.probe.grace_period", 60)
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.endpoint", "/injectlib")
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.container_registry", "")
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.default_dd_registries", []string{
		"gcr.io/datadoghq",
		"docker.io/datadog",
		"public.ecr.aws/datadog",
		"datadoghq.azurecr.io",
		"us-docker.pkg.dev/datadoghq/gcr.io",
		"europe-docker.pkg.dev/datadoghq/eu.gcr.io",
		"asia-docker.pkg.dev/datadoghq/asia.gcr.io",
		"registry.datad0g.com",
		"registry.datadoghq.com",
	})
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.gradual_rollout.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.gradual_rollout.cache_ttl", "1h")
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.patcher.enabled", false)
	// to be enabled only in e2e tests
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.patcher.fallback_to_file_provider", false)
	// to be used only in e2e tests
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.patcher.file_provider_path", "/etc/datadog-agent/patch/auto-instru.json")
	// allows injecting libraries for languages detected by automatic language detection feature
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.inject_auto_detected_libraries", true)
	// restricts which registries can be used for library injection
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.container_registry_allow_list", []string{})
	config.ParseEnvSplitComma("admission_controller.auto_instrumentation.container_registry_allow_list")
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.init_resources.cpu", "")
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.init_resources.memory", "")
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.init_security_context", "")
	// config for ASM which is implemented in the client libraries
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.asm.enabled", false, "DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_APPSEC_ENABLED")
	// config for IAST which is implemented in the client libraries
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.iast.enabled", false, "DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_IAST_ENABLED")
	// config for SCA
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.asm_sca.enabled", false, "DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_APPSEC_SCA_ENABLED")
	// config for profiling
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.profiling.enabled", "", "DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_PROFILING_ENABLED")
	config.BindEnvAndSetDefault("admission_controller.nccl_profiler.enabled", false, "DD_ADMISSION_CONTROLLER_NCCL_PROFILER_ENABLED")
	config.BindEnvAndSetDefault("admission_controller.nccl_profiler.injector_image", "", "DD_ADMISSION_CONTROLLER_NCCL_PROFILER_INJECTOR_IMAGE")
	// When true, the webhook injects into every pod that does not have the
	// opt-in label set to "false" (instead of requiring it to be "true").
	// Either this knob or the global admission_controller.mutate_unlabelled
	// switches blanket mode. Same shape as cws_instrumentation.
	config.BindEnvAndSetDefault("admission_controller.nccl_profiler.mutate_unlabelled", false)
	// Optional CPU/memory for the injected init container. Empty -> no Resources
	// block, cluster default applies. Same shape as cws_instrumentation.
	config.BindEnvAndSetDefault("admission_controller.nccl_profiler.init_resources.cpu", "")
	config.BindEnvAndSetDefault("admission_controller.nccl_profiler.init_resources.memory", "")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.enabled", false)
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.pod_endpoint", "/inject-pod-cws")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.command_endpoint", "/inject-command-cws")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.include", []string{})
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.exclude", []string{})
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.mutate_unlabelled", false)
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.container_registry", "")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.image_name", "cws-instrumentation")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.image_tag", "latest")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.init_resources.cpu", "")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.init_resources.memory", "")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.mode", "remote_copy")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.remote_copy.mount_volume", false)
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.remote_copy.directory", "/tmp")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.timeout", 2)
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.enabled", false)
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.provider", "")
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.endpoint", "/agentsidecar")
	// Should be able to parse it to a list of webhook selectors
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.selectors", "[]")
	// Should be able to parse it to a list of env vars and resource limits
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.profiles", "[]")
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.container_registry", "")
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.image_name", "agent")
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.image_tag", "latest")
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.cluster_agent.enabled", "true")
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.cluster_agent.tls_verification.enabled", false)
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.cluster_agent.tls_verification.copy_ca_configmap", false)
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.kubelet_api_logging.enabled", false)

	config.BindEnvAndSetDefault("admission_controller.kubernetes_admission_events.enabled", false)

	// Declare other keys that don't have a default/env var.
	// Mostly, keys we use IsSet() on, because IsSet always returns true if a key has a default.
	config.SetDefault("metadata_providers", []map[string]interface{}{})
	config.SetDefault("config_providers", []map[string]interface{}{})
	config.SetDefault("listeners", []map[string]interface{}{})

	config.BindEnvAndSetDefault("orchestrator_explorer.enabled", true)
	// enabling/disabling the environment variables & command scrubbing from the container specs
	// this option will potentially impact the CPU usage of the agent
	config.BindEnvAndSetDefault("orchestrator_explorer.container_scrubbing.enabled", true)
	config.BindEnvAndSetDefault("orchestrator_explorer.custom_resources.max_count", 5000)
	config.BindEnvAndSetDefault("orchestrator_explorer.custom_sensitive_words", []string{})
	config.BindEnvAndSetDefault("orchestrator_explorer.custom_sensitive_annotations_labels", []string{})
	config.BindEnvAndSetDefault("orchestrator_explorer.collector_discovery.enabled", true)
	config.BindEnvAndSetDefault("orchestrator_explorer.max_per_message", 100)
	config.BindEnvAndSetDefault("orchestrator_explorer.max_message_bytes", 10000000)
	config.BindEnvAndSetDefault("orchestrator_explorer.orchestrator_dd_url", "", "DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL", "DD_ORCHESTRATOR_URL")
	config.BindEnvAndSetDefault("orchestrator_explorer.orchestrator_additional_endpoints", map[string][]string{}, "DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_ADDITIONAL_ENDPOINTS", "DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS")
	config.BindEnvAndSetDefault("orchestrator_explorer.use_legacy_endpoint", false)
	config.BindEnvAndSetDefault("orchestrator_explorer.manifest_collection.enabled", true)
	config.BindEnvAndSetDefault("orchestrator_explorer.manifest_collection.buffer_manifest", true)
	config.BindEnvAndSetDefault("orchestrator_explorer.manifest_collection.buffer_flush_interval", 20*time.Second)
	config.BindEnvAndSetDefault("orchestrator_explorer.terminated_resources.enabled", true)
	config.BindEnvAndSetDefault("orchestrator_explorer.terminated_pods.enabled", true)
	config.BindEnvAndSetDefault("orchestrator_explorer.terminated_pods_improved.enabled", true)
	config.BindEnvAndSetDefault("orchestrator_explorer.custom_resources.ootb.enabled", true)
	config.BindEnvAndSetDefault("orchestrator_explorer.custom_resources.ootb.gateway_api", false)
	config.BindEnvAndSetDefault("orchestrator_explorer.custom_resources.ootb.service_mesh", false)
	config.BindEnvAndSetDefault("orchestrator_explorer.custom_resources.ootb.ingress_controllers", false)
	config.BindEnvAndSetDefault("orchestrator_explorer.kubelet_config_check.enabled", true, "DD_ORCHESTRATOR_EXPLORER_KUBELET_CONFIG_CHECK_ENABLED")
	config.BindEnvAndSetDefault("auto_team_tag_collection", true)

	config.BindEnvAndSetDefault("container_lifecycle.enabled", true)
	bindEnvAndSetLogsConfigKeys(config, "container_lifecycle.")

	config.BindEnvAndSetDefault("container_image.enabled", true)
	bindEnvAndSetLogsConfigKeys(config, "container_image.")

	// The interval at which processes are collected and sent to the workloadmeta in the core agent if the process
	// check is disabled.
	config.BindEnvAndSetDefault("workloadmeta.local_process_collector.collection_interval", 1*time.Minute)

	config.BindEnvAndSetDefault("sbom.enabled", false)
	bindEnvAndSetLogsConfigKeys(config, "sbom.")

	bindEnvAndSetLogsConfigKeys(config, "genresources.")

	config.BindEnvAndSetDefault("synthetics.collector.enabled", false)
	config.BindEnvAndSetDefault("synthetics.collector.workers", 4)
	config.BindEnvAndSetDefault("synthetics.collector.flush_interval", "10s")
	bindEnvAndSetLogsConfigKeys(config, "synthetics.forwarder.")

	config.BindEnvAndSetDefault("sbom.cache_directory", "${run_path}/sbom-agent")
	config.BindEnvAndSetDefault("sbom.compute_dependencies", true)
	config.BindEnvAndSetDefault("sbom.simplify_bom_refs", true)
	config.BindEnvAndSetDefault("sbom.clear_cache_on_exit", false)
	// used by custom cache: max disk space used by cached objects. Not equal to max disk usage
	config.BindEnvAndSetDefault("sbom.cache.max_disk_size", 1000*1000*100)
	// used by custom cache.
	config.BindEnvAndSetDefault("sbom.cache.clean_interval", "1h")
	config.BindEnvAndSetDefault("sbom.scan_queue.base_backoff", "5m")
	config.BindEnvAndSetDefault("sbom.scan_queue.max_backoff", "1h")

	config.BindEnvAndSetDefault("sbom.container_image.enabled", false)
	config.BindEnvAndSetDefault("sbom.container_image.use_mount", false)
	// Integer seconds
	config.BindEnvAndSetDefault("sbom.container_image.scan_interval", 0)
	// Integer seconds
	config.BindEnvAndSetDefault("sbom.container_image.scan_timeout", 10*60)
	config.BindEnvAndSetDefault("sbom.container_image.analyzers", []string{"os"})
	config.BindEnvAndSetDefault("sbom.container_image.check_disk_usage", true)
	config.BindEnvAndSetDefault("sbom.container_image.min_available_disk", "10Mb")
	config.BindEnvAndSetDefault("sbom.container_image.overlayfs_direct_scan", false)
	config.BindEnvAndSetDefault("sbom.container_image.overlayfs_disable_cache", true)
	config.BindEnvAndSetDefault("sbom.container_image.allow_missing_repodigest", false)
	config.BindEnvAndSetDefault("sbom.container_image.additional_directories", []string{})
	config.BindEnvAndSetDefault("sbom.container_image.use_spread_refresher", false)

	config.BindEnvAndSetDefault("sbom.container.enabled", false)

	config.BindEnvAndSetDefault("sbom.host.enabled", false)
	config.BindEnvAndSetDefault("sbom.host.analyzers", []string{"os"})
	config.BindEnvAndSetDefault("sbom.host.additional_directories", []string{})

	config.BindEnvAndSetDefault("sbom.enrichment.usage.enabled", false)

	bindEnvAndSetLogsConfigKeys(config, "service_discovery.forwarder.")

	// DEPRECATED in favor of `orchestrator_explorer.orchestrator_dd_url` setting. If both are set `orchestrator_explorer.orchestrator_dd_url` will take precedence.
	config.BindEnvAndSetDefault("process_config.orchestrator_dd_url", "", "DD_PROCESS_CONFIG_ORCHESTRATOR_DD_URL", "DD_PROCESS_AGENT_ORCHESTRATOR_DD_URL")
	// DEPRECATED in favor of `orchestrator_explorer.orchestrator_additional_endpoints` setting. If both are set `orchestrator_explorer.orchestrator_additional_endpoints` will take precedence.
	config.SetDefault("process_config.orchestrator_additional_endpoints", map[string][]string{})
	config.BindEnvAndSetDefault("orchestrator_explorer.extra_tags", []string{})

	config.BindEnvAndSetDefault("network.id", "")

	// Process manager (dd-procmgrd): on Windows the core agent starts dd-procmgr-service when enabled.
	// On Linux, dd-procmgrd is managed by systemd (datadog-agent-procmgr.service); this setting is ignored there.
	config.BindEnvAndSetDefault("process_manager.enabled", true)

	config.BindEnvAndSetDefault("otelcollector.enabled", false)
	config.BindEnvAndSetDefault("otelcollector.extension_url", "https://localhost:7777")
	// in seconds, 0 for default value
	config.BindEnvAndSetDefault("otelcollector.extension_timeout", 0)
	// dev flag - to be removed
	config.BindEnvAndSetDefault("otelcollector.submit_dummy_metadata", false)
	config.BindEnvAndSetDefault("otelcollector.converter.enabled", true)
	config.BindEnvAndSetDefault("otelcollector.flare.timeout", 60)
	config.BindEnvAndSetDefault("otelcollector.converter.features", []string{"infraattributes", "prometheus", "pprof", "zpages", "health_check", "ddflare", "datadog"})
	pkgconfighelper.ParseEnvSplitCommaAndSpace("otelcollector.converter.features", config)
	config.BindEnvAndSetDefault("otelcollector.gateway.mode", false)
	config.BindEnvAndSetDefault("otelcollector.installation_method", "")
	// otel_standalone controls whether otel-agent runs in standalone mode (with full secrets, tagger server)
	// or connected mode (expects core agent for secrets and tagger)
	config.BindEnvAndSetDefault("otel_standalone", false)

	config.BindEnvAndSetDefault("inventories_enabled", true)
	config.BindEnvAndSetDefault("inventories_configuration_enabled", true)
	config.BindEnvAndSetDefault("inventories_checks_configuration_enabled", true)
	config.BindEnvAndSetDefault("inventories_collect_cloud_provider_account_id", true)
	// when updating the default here also update pkg/metadata/inventories/README.md
	// 0 == default interval from inventories
	config.BindEnvAndSetDefault("inventories_max_interval", 0)
	// 0 == default interval from inventories
	config.BindEnvAndSetDefault("inventories_min_interval", 0)
	// Seconds to wait to sent metadata payload to the backend after startup
	config.BindEnvAndSetDefault("inventories_first_run_delay", 60)
	// resolve the hostname to get the IP address
	config.BindEnvAndSetDefault("metadata_ip_resolution_from_hostname", false)

	config.BindEnvAndSetDefault("security_agent.cmd_port", DefaultSecurityAgentCmdPort)
	config.BindEnvAndSetDefault("security_agent.expvar_port", 5011)
	config.BindEnvAndSetDefault("security_agent.log_file", "${log_path}/security-agent.log")
	config.BindEnvAndSetDefault("security_agent.disable_thp", true)

	// debug config to enable a remote client to receive data from the workloadmeta agent without a timeout
	config.BindEnvAndSetDefault("workloadmeta.remote.recv_without_timeout", true)

	config.BindEnvAndSetDefault("security_agent.internal_profiling.enabled", false, "DD_SECURITY_AGENT_INTERNAL_PROFILING_ENABLED")
	config.BindEnvAndSetDefault("security_agent.internal_profiling.site", DefaultSite, "DD_SECURITY_AGENT_INTERNAL_PROFILING_SITE", "DD_SITE")
	config.BindEnvAndSetDefault("security_agent.internal_profiling.profile_dd_url", "", "DD_SECURITY_AGENT_INTERNAL_PROFILING_DD_URL", "DD_APM_INTERNAL_PROFILING_DD_URL")
	config.BindEnvAndSetDefault("security_agent.internal_profiling.api_key", "", "DD_SECURITY_AGENT_INTERNAL_PROFILING_API_KEY", "DD_API_KEY")
	config.BindEnvAndSetDefault("security_agent.internal_profiling.env", "", "DD_SECURITY_AGENT_INTERNAL_PROFILING_ENV", "DD_ENV")
	config.BindEnvAndSetDefault("security_agent.internal_profiling.period", 5*time.Minute, "DD_SECURITY_AGENT_INTERNAL_PROFILING_PERIOD")
	config.BindEnvAndSetDefault("security_agent.internal_profiling.cpu_duration", 1*time.Minute, "DD_SECURITY_AGENT_INTERNAL_PROFILING_CPU_DURATION")
	config.BindEnvAndSetDefault("security_agent.internal_profiling.mutex_profile_fraction", 0)
	config.BindEnvAndSetDefault("security_agent.internal_profiling.block_profile_rate", 0)
	config.BindEnvAndSetDefault("security_agent.internal_profiling.enable_goroutine_stacktraces", false)
	config.BindEnvAndSetDefault("security_agent.internal_profiling.enable_block_profiling", false)
	config.BindEnvAndSetDefault("security_agent.internal_profiling.enable_mutex_profiling", false)
	config.BindEnvAndSetDefault("security_agent.internal_profiling.delta_profiles", true)
	config.BindEnvAndSetDefault("security_agent.internal_profiling.unix_socket", "")
	config.BindEnvAndSetDefault("security_agent.internal_profiling.extra_tags", []string{})

	config.BindEnvAndSetDefault("compliance_config.enabled", false)
	config.BindEnvAndSetDefault("compliance_config.run_in_system_probe", false)
	// deprecated, use host_benchmarks instead
	config.BindEnvAndSetDefault("compliance_config.xccdf.enabled", false)
	config.BindEnvAndSetDefault("compliance_config.host_benchmarks.enabled", true)
	config.BindEnvAndSetDefault("compliance_config.database_benchmarks.enabled", false)
	config.BindEnvAndSetDefault("compliance_config.check_interval", 20*time.Minute)
	config.BindEnvAndSetDefault("compliance_config.check_max_events_per_run", 100)
	config.BindEnvAndSetDefault("compliance_config.dir", "/etc/datadog-agent/compliance.d")
	config.BindEnvAndSetDefault("compliance_config.run_commands_as", "")
	bindEnvAndSetLogsConfigKeys(config, "compliance_config.endpoints.")
	config.BindEnvAndSetDefault("compliance_config.metrics.enabled", false)
	config.BindEnvAndSetDefault("compliance_config.opa.metrics.enabled", false)
	config.BindEnvAndSetDefault("compliance_config.container_include", []string{})
	config.BindEnvAndSetDefault("compliance_config.container_exclude", []string{})
	config.BindEnvAndSetDefault("compliance_config.exclude_pause_container", true)

	config.BindEnvAndSetDefault("runtime_security_config.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.socket", GetPlatformDefault(map[string]interface{}{
		"windows": "localhost:3335",
		"other":   "${install_path}/run/runtime-security.sock",
	}))
	config.BindEnvAndSetDefault("runtime_security_config.cmd_socket", "")
	config.BindEnvAndSetDefault("runtime_security_config.use_secruntime_track", true)
	bindEnvAndSetLogsConfigKeys(config, "runtime_security_config.endpoints.")
	bindEnvAndSetLogsConfigKeys(config, "runtime_security_config.activity_dump.remote_storage.endpoints.")
	config.BindEnvAndSetDefault("runtime_security_config.direct_send_from_system_probe", false)
	config.BindEnvAndSetDefault("runtime_security_config.event_grpc_server", "")

	config.SetDefault("cmd.check.fullsketches", false)

	// Windows Performance Counter refresh interval in seconds (introduced in 7.40, narrowed down
	// in 7.42). Additional information can be found where it is used (refreshPdhObjectCache())
	// The refresh can be disabled by setting the interval to 0.
	config.BindEnvAndSetDefault("windows_counter_refresh_interval", 60)

	// Added in Agent version 7.42
	// Limits the number of times a check will attempt to initialize a performance counter before ceasing
	// attempts to initialize the counter. This allows the Agent to stop incurring the overhead of trying
	// to initialize a counter that will probably never succeed. For example, when the performance counter
	// database needs to be rebuilt or the counter is disabled.
	// https://learn.microsoft.com/en-us/troubleshoot/windows-server/performance/manually-rebuild-performance-counters
	//
	// The value of this option should be chosen in consideration with the windows_counter_refresh_interval option.
	// The performance counter cache is refreshed during subsequent attempts to intiialize a counter that failed
	// the first time (with consideration of the windows_counter_refresh_interval value).
	// It is unknown if it is possible for a counter that failed to initialize to later succeed without a refresh
	// in between the attempts. Consequently, if windows_counter_refresh_interval is 0 (disabled), then this option should
	// be 1. If this option is too small compared to the windows_counter_refresh_interval, it is possible to reach the limit
	// before a refresh occurs. Typically there is one attempt per check run, and check runs are 15 seconds apart by default.
	//
	// Increasing this value may help in the rare instance where counters are not available for some time after host boot.
	//
	// Setting this option to 0 disables the limit and the Agent will attempt to initialize the counter forever.
	// The default value of 20 means the Agent will retry counter intialization for roughly 5 minutes.
	config.BindEnvAndSetDefault("windows_counter_init_failure_limit", 20)

	config.BindEnvAndSetDefault("system_tray.log_file", "${log_path}/ddtray.log")

	config.BindEnvAndSetDefault("language_detection.enabled", false)
	config.BindEnvAndSetDefault("language_detection.reporting.enabled", true)
	// buffer period represents how frequently newly detected languages buffer is flushed by reporting its content to the language detection handler in the cluster agent
	config.BindEnvAndSetDefault("language_detection.reporting.buffer_period", "10s")
	// TTL refresh period represents how frequently actively detected languages are refreshed by reporting them again to the language detection handler in the cluster agent
	config.BindEnvAndSetDefault("language_detection.reporting.refresh_period", "20m")

	// Appsec Proxy Config Injection (Experimental)
	config.BindEnvAndSetDefault("appsec.proxy.enabled", false)
	config.BindEnvAndSetDefault("appsec.proxy.processor.port", 443)
	config.BindEnvAndSetDefault("appsec.proxy.processor.address", "")
	config.BindEnvAndSetDefault("appsec.proxy.auto_detect", true)
	config.BindEnvAndSetDefault("appsec.proxy.proxies", []string{})

	config.BindEnvAndSetDefault("remote_updates", true)
	config.BindEnvAndSetDefault("installer.mirror", "")
	config.BindEnvAndSetDefault("installer.registry.url", "")
	config.BindEnvAndSetDefault("installer.registry.auth", "")
	config.BindEnvAndSetDefault("installer.registry.username", "")
	config.BindEnvAndSetDefault("installer.registry.password", "")
	config.BindEnvAndSetDefault("installer.refresh_interval", time.Duration(30*time.Second))
	config.BindEnvAndSetDefault("installer.gc_interval", time.Duration(time.Hour))

	// Legacy installer configuration
	config.SetDefault("remote_policies", false)

	config.BindEnvAndSetDefault("djm_config.enabled", false)

	bindEnvAndSetLogsConfigKeys(config, "data_observability.forwarder.")

	config.BindEnvAndSetDefault("reverse_dns_enrichment.workers", 10)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.chan_size", 5000)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.rate_limiter.enabled", true)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.cache.enabled", true)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.cache.entry_ttl", 24*time.Hour)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.cache.clean_interval", 2*time.Hour)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.cache.persist_interval", 2*time.Hour)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.cache.max_retries", 10)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.cache.max_size", 1000000)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.rate_limiter.limit_per_sec", 1000)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.rate_limiter.limit_throttled_per_sec", 1)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.rate_limiter.throttle_error_threshold", 10)
	// These variables are similarly named, but they serve different purposes:
	// - recovery_interval is the time to wait before trying to send data again after hitting the throttle_error_threshold
	// - recovery_intervals is the number of consecutive intervals with errors before considering the issue resolved and lifting the throttling
	config.BindEnvAndSetDefault("reverse_dns_enrichment.rate_limiter.recovery_interval", 5*time.Second)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.rate_limiter.recovery_intervals", 5)

	config.BindEnvAndSetDefault("remote_agent.registry.enabled", true)
	config.BindEnvAndSetDefault("remote_agent.registry.idle_timeout", time.Duration(30*time.Second))
	config.BindEnvAndSetDefault("remote_agent.registry.query_timeout", time.Duration(3*time.Second))
	config.BindEnvAndSetDefault("remote_agent.registry.recommended_refresh_interval", time.Duration(10*time.Second))
	config.BindEnvAndSetDefault("remote_agent.configstream.sleep_interval", 10*time.Second)
	config.BindEnvAndSetDefault("remote_agent.configstream.consumer.enabled", false)

	// Data Plane
	// NOTE: default temporarily flipped to true to run the full e2e suite with
	// ADP enabled (DADP-72). Do NOT merge — throwaway CI exploration only.
	config.BindEnvAndSetDefault("data_plane.enabled", true)
	config.BindEnvAndSetDefault("data_plane.use_new_config_stream_endpoint", true)
	config.BindEnvAndSetDefault("data_plane.remote_agent_enabled", true)
	// Listen addresses must include a URL scheme (e.g. "tcp://").
	config.BindEnvAndSetDefault("data_plane.api_listen_address", "tcp://0.0.0.0:5100")
	config.BindEnvAndSetDefault("data_plane.secure_api_listen_address", "tcp://0.0.0.0:5101")
	config.BindEnvAndSetDefault("data_plane.telemetry_enabled", false)
	config.BindEnvAndSetDefault("data_plane.telemetry_listen_addr", "tcp://0.0.0.0:5102")
	config.BindEnvAndSetDefault("data_plane.log_file", "${log_path}/agent-data-plane.log")
	// 4 matches aggregator_stop_timeout + forwarder_stop_timeout (each defaults to 2 in seconds).
	// ComputeDataPlaneStopTimeout (post-load override) recomputes this at runtime so it tracks
	// any user-set values for aggregator_stop_timeout / forwarder_stop_timeout.
	config.BindEnvAndSetDefault("data_plane.stop_timeout", 4)
	config.BindEnvAndSetDefault("data_plane.dogstatsd.enabled", true)
	config.BindEnvAndSetDefault("data_plane.otlp.enabled", false)
	config.BindEnvAndSetDefault("data_plane.otlp.proxy.enabled", false)
	config.BindEnvAndSetDefault("data_plane.otlp.proxy.traces.enabled", true)
	config.BindEnvAndSetDefault("data_plane.otlp.proxy.metrics.enabled", true)
	config.BindEnvAndSetDefault("data_plane.otlp.proxy.logs.enabled", true)
	// When the ADP OTLP proxy is enabled, ADP owns the gRPC endpoint configured for the receiver (default :4317) and the core agent uses the endpoint below
	config.BindEnvAndSetDefault("data_plane.otlp.proxy.receiver.protocols.grpc.endpoint", "127.0.0.1:4319")
	// ADP-specific zstd compression level, distinct from the core Agent's serializer_zstd_compressor_level
	// (default 1). ADP defaults to 3 for ~6% smaller payloads without a net CPU increase, since ADP is
	// more efficient than the Agent. Forwarded to ADP over the config stream.
	config.BindEnvAndSetDefault("data_plane.serializer_zstd_compressor_level", 3)

	config.BindEnvAndSetDefault("cel_workload_exclude", []interface{}{})

	config.BindEnvAndSetDefault("shared_library_check.enabled", false)
	config.BindEnvAndSetDefault("shared_library_check.library_folder_path", "${conf_path}/checks.d")

	config.BindEnvAndSetDefault("vsock_addr", "")

	// Delegated authentication (global)
	// Cloud provider and region are auto-detected if not specified
	// Enabled automatically when org_uuid is specified
	bindDelegatedAuthConfig(config, "")

	config.BindEnvAndSetDefault("metric_filterlist", []string{})
	config.BindEnvAndSetDefault("statsd_metric_blocklist", []string{})
	config.BindEnvAndSetDefault("metric_filterlist_match_prefix", false)
	config.BindEnvAndSetDefault("statsd_metric_blocklist_match_prefix", false)
	config.BindEnvAndSetDefault("metric_tag_filterlist", []interface{}{})
	// When true (default), metric_tag_filterlist tag stripping is only applied when data_plane.enabled is true.
	// Set to false to apply tag stripping regardless of whether ADP is running.
	config.BindEnvAndSetDefault("metric_tag_filterlist_adp_only", true)

	// When enabled, integrations will ignore configuration parameters that refer to file paths
	// Ignore file path params from untrusted providers (e.g. labels, annotations) when enabled.
	config.BindEnvAndSetDefault("integration_ignore_untrusted_file_params", false)

	// Allowlisted file paths for untrusted providers (empty = allow all).
	config.BindEnvAndSetDefault("integration_file_paths_allowlist", []string{})

	// Trusted config providers (others are untrusted). Defaults: file, remote-config.
	config.BindEnvAndSetDefault("integration_trusted_providers", []string{"file", "remote-config"})

	// Integrations excluded from these restrictions.
	config.BindEnvAndSetDefault("integration_security_excluded_checks", []string{})

	config.BindEnvAndSetDefault("hostprofiler.debug.verbosity", "")
	config.BindEnvAndSetDefault("hostprofiler.additional_http_headers", map[string]string{})
	config.BindEnvAndSetDefault("hostprofiler.ddprofiling.enabled", false)
	config.BindEnvAndSetDefault("hostprofiler.ddprofiling.period", 0)
	config.BindEnvAndSetDefault("hostprofiler.ddprofiling.port", 0)
	config.BindEnvAndSetDefault("hostprofiler.health_metrics.enabled", true)
	config.BindEnvAndSetDefault("hostprofiler.health_metrics.target", "127.0.0.1:8889")
	config.BindEnvAndSetDefault("hostprofiler.hpflare.port", 7778)
}

func agent(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("api_key", "")
	config.BindEnvAndSetDefault("site", DefaultSite)
	config.BindEnvAndSetDefault("convert_dd_site_fqdn.enabled", true)
	config.BindEnvAndSetDefault("dd_url", "https://app.datadoghq.com", "DD_DD_URL", "DD_URL")
	config.BindEnvAndSetDefault("app_key", "")
	config.BindEnvAndSetDefault("cloud_provider_metadata", []string{"aws", "gcp", "azure", "alibaba", "oracle", "ibm"})
	config.SetDefault("proxy.http", "")
	config.SetDefault("proxy.https", "")
	config.SetDefault("proxy.no_proxy", []string{})

	config.BindEnvAndSetDefault("skip_ssl_validation", false)
	config.BindEnvAndSetDefault("sslkeylogfile", "")
	config.BindEnvAndSetDefault("tls_handshake_timeout", 10*time.Second)
	config.BindEnvAndSetDefault("http_dial_fallback_delay", -1*time.Nanosecond)
	config.BindEnvAndSetDefault("hostname", "")
	config.BindEnvAndSetDefault("hostname_file", "")
	config.BindEnvAndSetDefault("tags", []string{})
	config.BindEnvAndSetDefault("extra_tags", []string{})
	// If enabled, all origin detection mechanisms will be unified to use the same logic.
	// Will override all other origin detection settings in favor of the unified one.
	config.BindEnvAndSetDefault("origin_detection_unified", false)
	config.BindEnvAndSetDefault("env", "")
	config.BindEnvAndSetDefault("tag_value_split_separator", map[string]string{})
	config.BindEnvAndSetDefault("conf_path", "${conf_path}")
	config.BindEnvAndSetDefault("confd_path", "${conf_path}/conf.d")
	config.BindEnvAndSetDefault("additional_checksd", "${conf_path}/checks.d")
	config.BindEnvAndSetDefault("jmx_log_file", "${log_path}/jmxfetch.log")
	// If enabling log_payloads, ensure the log level is set to at least DEBUG to be able to see the logs
	config.BindEnvAndSetDefault("log_payloads", false)
	config.BindEnvAndSetDefault("log_file", "${log_path}/agent.log")
	config.BindEnvAndSetDefault("log_file_max_size", "10Mb")
	config.BindEnvAndSetDefault("log_file_max_rolls", 1)
	config.BindEnvAndSetDefault("log_level", "info")
	config.BindEnvAndSetDefault("log_to_syslog", false)
	config.BindEnvAndSetDefault("log_to_console", true)
	config.BindEnvAndSetDefault("log_format_rfc3339", false)
	config.BindEnvAndSetDefault("log_all_goroutines_when_unhealthy", false)
	config.BindEnvAndSetDefault("logging_frequency", int64(500))
	config.BindEnvAndSetDefault("disable_file_logging", false)
	config.BindEnvAndSetDefault("syslog_uri", "")
	config.BindEnvAndSetDefault("syslog_rfc", false)
	// deprecated: use `cmd_host` instead
	config.BindEnvAndSetDefault("ipc_address", "localhost")
	config.BindEnvAndSetDefault("cmd_host", "localhost")
	config.BindEnvAndSetDefault("cmd_port", 5001)
	config.BindEnvAndSetDefault("agent_ipc.socket_path", "${run_path}/agent_ipc.socket")
	config.BindEnvAndSetDefault("agent_ipc.use_socket", false)
	config.BindEnvAndSetDefault("agent_ipc.host", "localhost")
	config.BindEnvAndSetDefault("agent_ipc.port", 0)
	config.BindEnvAndSetDefault("agent_ipc.config_refresh_interval", 0)
	config.BindEnvAndSetDefault("agent_ipc.grpc_max_message_size", 128<<20)
	config.BindEnvAndSetDefault("agent_ipc.grpc_warning_message_size", 32<<20)
	config.BindEnvAndSetDefault("default_integration_http_timeout", 9)
	config.BindEnvAndSetDefault("integration_tracing", false)
	config.BindEnvAndSetDefault("integration_tracing_exhaustive", false)
	config.BindEnvAndSetDefault("integration_profiling", false)
	config.BindEnvAndSetDefault("integration_check_status_enabled", false)
	config.BindEnvAndSetDefault("enable_metadata_collection", true)
	config.BindEnvAndSetDefault("enable_cluster_agent_metadata_collection", true)
	config.BindEnvAndSetDefault("enable_gohai", true)
	config.BindEnvAndSetDefault("enable_signing_metadata_collection", true)
	config.BindEnvAndSetDefault("metadata_provider_stop_timeout", 30*time.Second)
	config.BindEnvAndSetDefault("inventories_diagnostics_enabled", true)
	config.BindEnvAndSetDefault("check_runners", int64(4))
	config.BindEnvAndSetDefault("check_cancel_timeout", 500*time.Millisecond)
	config.BindEnvAndSetDefault("check_runner_utilization_threshold", float64(0.95))
	config.BindEnvAndSetDefault("check_runner_utilization_monitor_interval", 60*time.Second)
	config.BindEnvAndSetDefault("check_runner_utilization_warning_cooldown", 10*time.Minute)
	config.BindEnvAndSetDefault("check_system_probe_startup_time", 5*time.Minute)
	config.BindEnvAndSetDefault("check_system_probe_timeout", 60*time.Second)
	// If not zero, the agent will log a warning if a check is running for longer than this timeout
	config.BindEnvAndSetDefault("check_watchdog_warning_timeout", 0*time.Second)
	config.BindEnvAndSetDefault("auth_token_file_path", "")
	// used to override the path where the IPC cert/key files are stored/retrieved
	config.BindEnvAndSetDefault("ipc_cert_file_path", "")
	// used to override the acceptable duration for the agent to load or create auth artifacts (auth_token and IPC cert/key files)
	config.BindEnvAndSetDefault("auth_init_timeout", 30*time.Second)
	config.BindEnvAndSetDefault("bind_host", "")
	config.BindEnvAndSetDefault("health_port", int64(0))
	config.BindEnvAndSetDefault("health_platform.enabled", true)
	config.BindEnvAndSetDefault("health_platform.persist_on_kubernetes", false)
	config.BindEnvAndSetDefault("health_platform.forwarder.interval", 15*time.Minute)
	config.BindEnvAndSetDefault("health_platform.invalidconfig_check.enabled", true)
	config.BindEnvAndSetDefault("health_platform.invalidsysprobeconfig_check.enabled", true)
	config.BindEnvAndSetDefault("disable_py3_validation", false)
	config.BindEnvAndSetDefault("win_skip_com_init", false)
	config.BindEnvAndSetDefault("allow_arbitrary_tags", false)
	config.BindEnvAndSetDefault("use_proxy_for_cloud_metadata", false)

	// Legacy alias for backward compatibility
	// This applies to the current infrastructure_mode
	config.BindEnvAndSetDefault("allowed_additional_checks", []string{})

	config.BindEnvAndSetDefault("integration.enabled", true)

	// integration.additional: additional checks to allow beyond the default set (user configured)
	config.BindEnvAndSetDefault("integration.additional", []string{})
	// integration.excluded: checks to exclude (user configured)
	config.BindEnvAndSetDefault("integration.excluded", []string{})

	// Infrastructure mode
	// The infrastructure mode is used to determine the features that are available to the agent.
	// The possible values are: full, basic, end_user_device, cloud_cost_only, none.
	config.BindEnvAndSetDefault("infrastructure_mode", "full")

	// Infrastructure full mode section (default mode, allows all checks)
	// integration.full.allowed: empty means all checks are allowed
	config.BindEnvAndSetDefault("integration.full.allowed", []string{})

	// Infrastructure end_user_device mode section
	// integration.end_user_device.allowed: empty means all checks are allowed
	config.BindEnvAndSetDefault("integration.end_user_device.allowed", []string{})

	// integration.cloud_cost_only.tagged: checks to tag when infrastructure_mode=cloud_cost_only (empty means all checks)
	config.BindEnvAndSetDefault("integration.cloud_cost_only.tagged", []string{})

	// Infrastructure basic mode section [UNDOCUMENTED]
	// Note: All checks starting with "custom_" are always allowed.
	// integration.basic.allowed: default allowed checks (internal, should not need user configuration)
	config.BindEnvAndSetDefault("integration.basic.allowed", []string{
		"cpu",
		"agent_telemetry",
		"agentcrashdetect",
		"disk",
		"directory",
		"file_handle",
		"filehandles",
		"io",
		"load",
		"memory",
		"network",
		"ntp",
		"process",
		"service_discovery",
		"snmp",
		"cisco_sdwan",
		"versa",
		"cisco_aci",
		"system",
		"systemd",
		"system_core",
		"system_swap",
		"telemetry",
		"telemetryCheck",
		"uptime",
		"win32_event_log",
		"wincrashdetect",
		"winkmem",
		"winproc",
		"wmi_check",
		"windows_certificate",
		"windows_performance_counters",
		"windows_registry",
		"windows_service",
	})
	// Configuration for TLS for outgoing connections
	config.BindEnvAndSetDefault("min_tls_version", "tlsv1.2")

	// Use to output logs in JSON format
	config.BindEnvAndSetDefault("log_format_json", false)

	// Yaml keys which values are stripped from flare
	config.BindEnvAndSetDefault("flare_stripped_keys", []string{})
	config.BindEnvAndSetDefault("scrubber.additional_keys", []string{})

	// Duration during which the host tags will be submitted with metrics.
	config.BindEnvAndSetDefault("expected_tags_duration", time.Duration(0))

	// Agent GUI access host
	// 		'http://localhost' is preferred over 'http://127.0.0.1' due to Internet Explorer behavior.
	// 		Internet Explorer High Security Level does not support setting cookies via HTTP Header response.
	// 		By default, 'http://localhost' is categorized as an "intranet" website, which is considered safer and allowed to use cookies. This is not the case for 'http://127.0.0.1'.
	config.BindEnvAndSetDefault("GUI_host", "localhost")
	// Agent GUI access port
	config.BindEnvAndSetDefault("GUI_port", GetPlatformDefault(map[string]interface{}{
		"darwin":  5002,
		"windows": 5002,
		"other":   -1,
	}))
	config.BindEnvAndSetDefault("GUI_session_expiration", 0)

	// Core agent (disabled for Error Tracking Standalone, Logs Collection Only)
	config.BindEnvAndSetDefault("core_agent.enabled", true)

	config.BindEnvAndSetDefault("config_files_discovery.enabled", false)
	bindEnvAndSetLogsConfigKeys(config, "config_files_discovery.forwarder.")

	config.BindEnvAndSetDefault("software_inventory.enabled", false)
	config.BindEnvAndSetDefault("software_inventory.jitter", 60)
	config.BindEnvAndSetDefault("software_inventory.interval", 10)
	bindEnvAndSetLogsConfigKeys(config, "software_inventory.forwarder.")

	config.BindEnvAndSetDefault("notable_events.enabled", false)

	config.BindEnvAndSetDefault("logon_duration.enabled", false)

	// Event Management v2 API
	// https://docs.datadoghq.com/api/latest/events#post-an-event
	bindEnvAndSetLogsConfigKeys(config, "event_management.forwarder.")

	// The cardinality of tags to send for checks.
	// Choices are: low, orchestrator, high.
	// Changing this setting may impact your custom metrics billing.
	config.BindEnvAndSetDefault("checks_tag_cardinality", "low")

	// The cardinality of tags to send for dogstatsd.
	// Choices are: low, orchestrator, high.
	// WARNING: sending orchestrator, or high tags for dogstatsd metrics may create more metrics
	// (one per container instead of one per host).
	// Changing this setting may impact your custom metrics billing.
	config.BindEnvAndSetDefault("dogstatsd_tag_cardinality", "low")

	config.BindEnvAndSetDefault("cel_workload_exclude", []interface{}{})

	config.BindEnvAndSetDefault("sbom.container_image.container_include", []string{})
	config.BindEnvAndSetDefault("sbom.container_image.container_exclude", []string{})
	config.BindEnvAndSetDefault("ecs_collect_resource_tags_ec2", false)
	config.BindEnvAndSetDefault("docker_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("docker_env_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_node_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_namespace_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_namespace_annotations_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_pod_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_pod_annotations_as_tags", map[string]string{})
	// kubernetes_resources_labels_as_tags should be parseable as map[string]map[string]string
	// it maps group resources to labels as tags maps
	// a group resource has the format `{resource}.{group}`, or simply `{resource}` if it belongs to the empty group
	// examples of group resources:
	// 	- `deployments.apps`
	// 	- `statefulsets.apps`
	// 	- `pods`
	// 	- `nodes`
	config.BindEnvAndSetDefault("kubernetes_resources_labels_as_tags", "{}")
	// kubernetes_resources_annotations_as_tags should be parseable as map[string]map[string]string
	// it maps group resources to annotations as tags maps
	// a group resource has the format `{resource}.{group}`, or simply `{resource}` if it belongs to the empty group
	// examples of group resources:
	// 	- `deployments.apps`
	// 	- `statefulsets.apps`
	// 	- `pods`
	// 	- `nodes`
	config.BindEnvAndSetDefault("kubernetes_resources_annotations_as_tags", "{}")
	config.BindEnvAndSetDefault("provider_kind", "")

	config.BindEnvAndSetDefault("sbom.container_image.exclude_pause_container", true)
	config.BindEnvAndSetDefault("kubernetes_persistent_volume_claims_as_tags", true)
	config.BindEnvAndSetDefault("kubernetes_node_annotations_as_tags", map[string]string{"cluster.k8s.io/machine": "kube_machine"})
}

func fleet(config pkgconfigmodel.Setup) {
	// Directory to store fleet policies
	config.BindEnvAndSetDefault("fleet_policies_dir", "")
	config.SetDefault("fleet_layers", []string{})
	config.BindEnvAndSetDefault("config_id", "")
}

func autoscaling(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("autoscaling.workload.enabled", false)
	config.BindEnvAndSetDefault("autoscaling.failover.enabled", false)
	config.BindEnvAndSetDefault("autoscaling.workload.limit", 1000)
	config.BindEnvAndSetDefault("autoscaling.workload.num_workers", 2)
	// Enables the external recommender feature.
	config.BindEnvAndSetDefault("autoscaling.workload.external_recommender.enabled", false)
	config.BindEnvAndSetDefault("autoscaling.workload.external_recommender.tls.ca_file", "")
	config.BindEnvAndSetDefault("autoscaling.workload.external_recommender.tls.cert_file", "")
	config.BindEnvAndSetDefault("autoscaling.workload.external_recommender.tls.key_file", "")
	config.BindEnvAndSetDefault("autoscaling.workload.in_place_vertical_scaling.enabled", true)
	config.BindEnvAndSetDefault("autoscaling.workload.in_place_vertical_scaling.disruption_tolerance_percent", 15)
	config.BindEnvAndSetDefault("autoscaling.failover.metrics", []string{"container.memory.usage", "container.cpu.usage"})

	config.BindEnvAndSetDefault("autoscaling.cluster.enabled", false)

	config.BindEnvAndSetDefault("autoscaling.cluster.spot.enabled", false)
	config.BindEnvAndSetDefault("autoscaling.cluster.spot.defaults.percentage", 100)
	config.BindEnvAndSetDefault("autoscaling.cluster.spot.defaults.min_on_demand_replicas", 0)
	config.BindEnvAndSetDefault("autoscaling.cluster.spot.schedule_timeout", "1m")
	config.BindEnvAndSetDefault("autoscaling.cluster.spot.fallback_duration", "2m")
	config.BindEnvAndSetDefault("autoscaling.cluster.spot.rebalance_stabilization_period", "1m")

	config.BindEnvAndSetDefault("kubeactions.enabled", false)
	// TODO(kubeactions): Update hostnameEndpointPrefix to "kubeops-intake." once provisioned
	bindEnvAndSetLogsConfigKeys(config, "kubeactions.forwarder.")
}

func fips(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("fips.enabled", false)
	config.BindEnvAndSetDefault("fips.port_range_start", 9803)
	config.BindEnvAndSetDefault("fips.local_address", "localhost")
	config.BindEnvAndSetDefault("fips.https", true)
	config.BindEnvAndSetDefault("fips.tls_verify", true)
}

func remoteconfig(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("remote_configuration.enabled", true)
	config.BindEnvAndSetDefault("remote_configuration.key", "")
	config.BindEnvAndSetDefault("remote_configuration.api_key", "")
	config.BindEnvAndSetDefault("remote_configuration.rc_dd_url", "")
	// Delegated authentication for remote_configuration
	bindDelegatedAuthConfig(config, "remote_configuration")
	config.BindEnvAndSetDefault("remote_configuration.no_tls", false)
	config.BindEnvAndSetDefault("remote_configuration.no_tls_validation", false)
	config.BindEnvAndSetDefault("remote_configuration.config_root", "")
	config.BindEnvAndSetDefault("remote_configuration.director_root", "")
	config.BindEnvAndSetDefault("remote_configuration.refresh_interval", "0s")
	config.BindEnvAndSetDefault("remote_configuration.org_status_refresh_interval", 1*time.Minute)
	config.BindEnvAndSetDefault("remote_configuration.max_backoff_interval", 2*time.Minute)
	config.BindEnvAndSetDefault("remote_configuration.clients.ttl_seconds", 30*time.Second)
	config.BindEnvAndSetDefault("remote_configuration.clients.cache_bypass_limit", 5)
	config.BindEnvAndSetDefault("remote_configuration.apm_sampling.enabled", true)
	// agent_config.enabled gates the trace-agent's AGENT_CONFIG subscription.
	// Defaults to true. The trace-agent additionally inherits
	// remote_configuration.apm_sampling.enabled when the user has explicitly
	// set apm_sampling.enabled but NOT agent_config.enabled, preserving the
	// historical behavior where apm_sampling.enabled implicitly gated
	// AGENT_CONFIG too.
	config.BindEnvAndSetDefault("remote_configuration.agent_config.enabled", true)
	// apm_semantics.enabled gates the trace-agent's APM_SEMANTIC_CORE_DD
	// subscription. Opt-in during initial rollout.
	config.BindEnvAndSetDefault("remote_configuration.apm_semantics.enabled", false)
	config.BindEnvAndSetDefault("remote_configuration.agent_integrations.enabled", false)
	config.BindEnvAndSetDefault("remote_configuration.agent_integrations.allow_list", defaultAllowedRCIntegrations)
	config.BindEnvAndSetDefault("remote_configuration.agent_integrations.block_list", []string{})
	config.BindEnvAndSetDefault("remote_configuration.agent_integrations.allow_log_config_scheduling", false)
	// Websocket echo test
	config.BindEnvAndSetDefault("remote_configuration.no_websocket_echo", false)
}

func remoteflags(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("remote_flags.enabled", false)
}

func autoconfig(config pkgconfigmodel.Setup) {
	// Where to look for check templates if no custom path is defined
	config.BindEnvAndSetDefault("autoconf_template_dir", "/datadog/check_configs")
	config.BindEnvAndSetDefault("autoconf_config_files_poll", false)
	config.BindEnvAndSetDefault("autoconf_config_files_poll_interval", 60)
	config.BindEnvAndSetDefault("exclude_pause_container", true)
	config.BindEnvAndSetDefault("include_ephemeral_containers", false)
	config.BindEnvAndSetDefault("ac_include", []string{})
	config.BindEnvAndSetDefault("ac_exclude", []string{})
	config.BindEnvAndSetDefault("container_include", []string{})
	config.BindEnvAndSetDefault("container_exclude", []string{})
	config.BindEnvAndSetDefault("container_include_metrics", []string{})
	config.BindEnvAndSetDefault("container_exclude_metrics", []string{})
	config.BindEnvAndSetDefault("container_include_logs", []string{})
	config.BindEnvAndSetDefault("container_exclude_logs", []string{})
	// in hours
	config.BindEnvAndSetDefault("container_exclude_stopped_age", DefaultAuditorTTL-1)
	// in seconds
	config.BindEnvAndSetDefault("ad_config_poll_interval", int64(10))
	// in seconds, 0 means disabled
	config.BindEnvAndSetDefault("ad_tag_completeness_max_wait", 0)
	config.BindEnvAndSetDefault("ad_allowed_env_vars", []string{})
	config.BindEnvAndSetDefault("ad_disable_env_var_resolution", false)
	config.BindEnvAndSetDefault("extra_listeners", []string{})
	config.BindEnvAndSetDefault("extra_config_providers", []string{})
	config.BindEnvAndSetDefault("ignore_autoconf", []string{})
	config.BindEnvAndSetDefault("autoconfig_from_environment", true)
	config.BindEnvAndSetDefault("autoconfig_exclude_features", []string{})
	config.BindEnvAndSetDefault("autoconfig_include_features", []string{})
}

func containerSyspath(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("procfs_path", "")
	// The correct default is deduce at runtime by scanning for mounted volumes (see fixupContainerSyspath).
	// The value here is a generic place holder used for our configuration examples.
	config.BindEnvAndSetDefault("container_proc_root", "/host/proc")
	// The correct default is deduce at runtime by scanning for mounted volumes (see fixupContainerSyspath).
	// The value here is a generic place holder used for our configuration examples.
	config.BindEnvAndSetDefault("container_cgroup_root", "/host/sys/fs/cgroup/")
	config.BindEnvAndSetDefault("container_pid_mapper", "")

	config.BindEnvAndSetDefault("ignore_host_etc", false)
	config.BindEnvAndSetDefault("use_improved_cgroup_parser", false)
	config.BindEnvAndSetDefault("proc_root", "/proc")
}

func debugging(config pkgconfigmodel.Setup) {
	// Debugging + C-land crash feature flags
	config.BindEnvAndSetDefault("c_stacktrace_collection", false)
	config.BindEnvAndSetDefault("c_core_dump", false)
	config.BindEnvAndSetDefault("go_core_dump", false)
	config.BindEnvAndSetDefault("memtrack_enabled", false)
	config.BindEnvAndSetDefault("tracemalloc_debug", false)
	config.BindEnvAndSetDefault("tracemalloc_include", "")
	config.BindEnvAndSetDefault("tracemalloc_exclude", "")
	// deprecated
	config.BindEnvAndSetDefault("tracemalloc_whitelist", "")
	// deprecated
	config.BindEnvAndSetDefault("tracemalloc_blacklist", "")
	config.BindEnvAndSetDefault("run_path", "${run_path}")
	config.BindEnvAndSetDefault("no_proxy_nonexact_match", false)
}

func telemetry(config pkgconfigmodel.Setup) {
	// Enable telemetry metrics on the internals of the Agent.
	// This create a lot of billable custom metrics.
	config.BindEnvAndSetDefault("telemetry.enabled", false)
	config.BindEnvAndSetDefault("telemetry.dogstatsd_origin", false)
	config.BindEnvAndSetDefault("telemetry.python_memory", true)
	config.BindEnvAndSetDefault("telemetry.checks", []string{})
	// We're using []string as a default instead of []float64 because viper can only parse list of string from the environment
	//
	// The histogram buckets use to track the time in nanoseconds DogStatsD listeners are not reading/waiting new data
	config.BindEnvAndSetDefault("telemetry.dogstatsd.listeners_latency_buckets", []string{})
	// The histogram buckets use to track the time in nanoseconds it takes for the DogStatsD server to push data to the aggregator
	config.BindEnvAndSetDefault("telemetry.dogstatsd.aggregator_channel_latency_buckets", []string{})
	// The histogram buckets use to track the time in nanoseconds it takes for a DogStatsD listeners to push data to the server
	config.BindEnvAndSetDefault("telemetry.dogstatsd.listeners_channel_latency_buckets", []string{})
	config.BindEnvAndSetDefault("telemetry.offlinereporter.enabled", false)
	config.BindEnvAndSetDefault("telemetry.offlinereporter.heartbeat_interval", "5s")

	config.BindEnvAndSetDefault("agent_telemetry.enabled", true)
	// default compression first setup inside the next bindEnvAndSetLogsConfigKeys() function ...
	bindEnvAndSetLogsConfigKeys(config, "agent_telemetry.")
	// ... and overridden by the following two lines - do not switch these 3 lines order
	config.BindEnvAndSetDefault("agent_telemetry.compression_level", 1)
	config.BindEnvAndSetDefault("agent_telemetry.use_compression", true)
	config.BindEnvAndSetDefault("agent_telemetry.startup_trace_sampling", 0)

	// experimental error log forwarding to telemetry. Use-sites must
	// additionally gate on pkg/config/utils.IsAgentTelemetryEnabled so
	// gov/FIPS exclusion is inherited from the parent agent_telemetry flag.
	config.BindEnvAndSetDefault("agent_telemetry.errortracking.enabled", false)
	config.BindEnvAndSetDefault("agent_telemetry.errortracking.bouncer_window_seconds", 900)
	config.BindEnvAndSetDefault("agent_telemetry.errortracking.flush_interval_seconds", 60)
	config.BindEnvAndSetDefault("agent_telemetry.errortracking.buffer_size", 2048)
	config.BindEnvAndSetDefault("agent_telemetry.errortracking.startup_jitter_seconds", 0)
	config.BindEnvAndSetDefault("agent_telemetry.errortracking.shutdown_drain_timeout_seconds", 5)
}

func serializer(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("enable_json_stream_shared_compressor_buffers", true)

	// Warning: do not change the following values. Your payloads will get dropped by Datadog's intake.
	config.BindEnvAndSetDefault("serializer_max_payload_size", 2*megaByte+megaByte/2)
	config.BindEnvAndSetDefault("serializer_max_uncompressed_payload_size", 4*megaByte)
	config.BindEnvAndSetDefault("serializer_max_series_points_per_payload", 10000)
	config.BindEnvAndSetDefault("serializer_max_series_payload_size", 512000)
	config.BindEnvAndSetDefault("serializer_max_series_uncompressed_payload_size", 5242880)
	config.BindEnvAndSetDefault("serializer_compressor_kind", DefaultCompressorKind)
	config.BindEnvAndSetDefault("serializer_zstd_compressor_level", DefaultZstdCompressionLevel)
	config.BindEnvAndSetDefault("serializer_experimental_use_v3_api.series.endpoints", []string{})
	config.BindEnvAndSetDefault("serializer_experimental_use_v3_api.sketches.endpoints", []string{})
	config.BindEnvAndSetDefault("serializer_experimental_use_v3_api.series.validate", false)
	config.BindEnvAndSetDefault("serializer_experimental_use_v3_api.sketches.validate", false)
	config.BindEnvAndSetDefault("serializer_experimental_use_v3_api.compression_level", 0)
	config.BindEnvAndSetDefault("serializer_experimental_use_v3_api.series.use_beta", false)
	config.BindEnvAndSetDefault("serializer_experimental_use_v3_api.series.beta_route", "/api/intake/metrics/v3beta/series")
	config.BindEnvAndSetDefault("serializer_experimental_use_v3_api.series.shadow_sample_rate", float64(0))
	config.BindEnvAndSetDefault("serializer_experimental_use_v3_api.series.shadow_sites", []string{"datadoghq.com"})

	config.BindEnvAndSetDefault("use_v3_api.series.enabled", "datadog_only")
	config.BindEnvAndSetDefault("use_v3_api.series.endpoints", map[string]string{})

	config.BindEnvAndSetDefault("use_v2_api.series", true)
	// Serializer: allow user to blacklist any kind of payload to be sent
	config.BindEnvAndSetDefault("enable_payloads.events", true)
	config.BindEnvAndSetDefault("enable_payloads.series", true)
	config.BindEnvAndSetDefault("enable_payloads.service_checks", true)
	config.BindEnvAndSetDefault("enable_payloads.sketches", true)
	config.BindEnvAndSetDefault("enable_payloads.json_to_v1_intake", true)
}

func aggregator(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("aggregator_stop_timeout", 2)
	config.BindEnvAndSetDefault("aggregator_buffer_size", 100)
	config.BindEnvAndSetDefault("aggregator_use_tags_store", true)
	config.BindEnvAndSetDefault("aggregator_tag_filter_cache_capacity", 1000)
	// ADP can cache more efficiently so we use a higher default
	config.BindEnvAndSetDefault("data_plane.dogstatsd.aggregator_tag_filter_cache_capacity", 100000)
	// configure adding the agent container tags to the basic agent telemetry metrics (e.g. `datadog.agent.running`)
	config.BindEnvAndSetDefault("basic_telemetry_add_container_tags", false)
	config.BindEnvAndSetDefault("aggregator_flush_metrics_and_serialize_in_parallel_chan_size", 200)
	config.BindEnvAndSetDefault("aggregator_flush_metrics_and_serialize_in_parallel_buffer_size", 4000)
}

func serverless(config pkgconfigmodel.Setup) {
	config.SetDefault("serverless.enabled", false)
	config.BindEnvAndSetDefault("serverless.logs_enabled", true)
	config.BindEnvAndSetDefault("enhanced_metrics", true, "DD_ENHANCED_METRICS_ENABLED")
	config.BindEnvAndSetDefault("serverless.trace_enabled", true, "DD_TRACE_ENABLED")
	config.BindEnvAndSetDefault("serverless.trace_managed_services", true, "DD_TRACE_MANAGED_SERVICES")
	config.BindEnvAndSetDefault("serverless.service_mapping", "", "DD_SERVICE_MAPPING")
}

func forwarder(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("additional_endpoints", map[string][]string{})
	config.BindEnvAndSetDefault("forwarder_timeout", 20)
	// Deprecated in favor of `forwarder_retry_queue_payloads_max_size`
	config.BindEnvAndSetDefault("forwarder_retry_queue_max_size", 0)
	config.BindEnvAndSetDefault("forwarder_retry_queue_payloads_max_size", 15*1024*1024)
	// in seconds, 0 means disabled
	config.BindEnvAndSetDefault("forwarder_connection_reset_interval", 0)
	// in minutes
	config.BindEnvAndSetDefault("forwarder_apikey_validation_interval", DefaultAPIKeyValidationInterval)
	config.BindEnvAndSetDefault("forwarder_num_workers", 1)
	config.BindEnvAndSetDefault("forwarder_stop_timeout", 2)
	// forwarder_stop_wait_for_inflight controls whether Worker.Stop waits for
	// in-flight HTTP transactions to finish before returning (true) or cancels
	// them immediately (false).
	config.BindEnvAndSetDefault("forwarder_stop_wait_for_inflight", false)
	config.BindEnvAndSetDefault("forwarder_max_concurrent_requests", 10)
	config.BindEnvAndSetDefault("forwarder_backoff_factor", 2)
	config.BindEnvAndSetDefault("forwarder_backoff_base", 2)
	config.BindEnvAndSetDefault("forwarder_backoff_max", 64)
	config.BindEnvAndSetDefault("forwarder_recovery_interval", DefaultForwarderRecoveryInterval)
	config.BindEnvAndSetDefault("forwarder_recovery_reset", false)

	config.BindEnvAndSetDefault("forwarder_storage_path", "${run_path}/transactions_to_retry")
	config.BindEnvAndSetDefault("forwarder_outdated_file_in_days", 10)
	config.BindEnvAndSetDefault("forwarder_flush_to_disk_mem_ratio", float64(0.5))
	// 0 means disabled. This is a BETA feature.
	config.BindEnvAndSetDefault("forwarder_storage_max_size_in_bytes", 0)
	// Do not store transactions on disk when the disk usage exceeds 80% of the disk capacity. Use 80% as some applications do not behave well when the disk space is very small.
	config.BindEnvAndSetDefault("forwarder_storage_max_disk_ratio", float64(0.80))
	// 15 mins
	config.BindEnvAndSetDefault("forwarder_retry_queue_capacity_time_interval_sec", 900)

	config.BindEnvAndSetDefault("forwarder_high_prio_buffer_size", 100)
	config.BindEnvAndSetDefault("forwarder_low_prio_buffer_size", 100)
	config.BindEnvAndSetDefault("forwarder_requeue_buffer_size", 100)
	config.BindEnvAndSetDefault("forwarder_http_protocol", "auto")
}

func dogstatsd(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("use_dogstatsd", true)
	// Notice: 0 means UDP port closed
	config.BindEnvAndSetDefault("dogstatsd_port", 8125)
	config.BindEnvAndSetDefault("dogstatsd_pipe_name", "")
	// When true, DogStatsD fails to start (the process exits non-zero) when no listener
	// (UDP port, UDS socket, or named pipe) could be created. When false, the failure is
	// only logged and the process keeps running.
	config.BindEnvAndSetDefault("dogstatsd_require_listener", false)
	// https://learn.microsoft.com/en-us/windows/win32/secauthz/security-descriptor-string-format
	// https://learn.microsoft.com/en-us/windows/win32/secauthz/ace-strings
	// https://learn.microsoft.com/en-us/windows/win32/secauthz/sid-strings
	//
	// D:dacl_flags(ace_type;ace_flags;rights;object_guid;inherit_object_guid;account_sid;(resource_attribute))
	// 	dacl_flags:
	//		"AI": SDDL_AUTO_INHERITED
	//	ace_type:
	//		"A": SDDL_ACCESS_ALLOWED
	// rights:
	//		"GA": SDDL_GENERIC_ALL
	// account_sid:
	//		"WD": Everyone
	config.BindEnvAndSetDefault("dogstatsd_windows_pipe_security_descriptor", "D:AI(A;;GA;;;WD)")
	// Options are: udp, uds, named_pipe
	config.BindEnvAndSetDefault("dogstatsd_eol_required", []string{})

	// The following options allow to configure how the dogstatsd intake buffers and queues incoming datagrams.
	// When a datagram is received it is first added to a datagrams buffer. This buffer fills up until
	// we reach `dogstatsd_packet_buffer_size` datagrams or after `dogstatsd_packet_buffer_flush_timeout` ms.
	// After this happens we flush this buffer of datagrams to a queue for processing. The size of this queue
	// is `dogstatsd_queue_size`.
	config.BindEnvAndSetDefault("dogstatsd_buffer_size", 1024*8)
	config.BindEnvAndSetDefault("dogstatsd_packet_buffer_size", 32)
	config.BindEnvAndSetDefault("dogstatsd_packet_buffer_flush_timeout", 100*time.Millisecond)
	config.BindEnvAndSetDefault("dogstatsd_queue_size", 1024)

	config.BindEnvAndSetDefault("dogstatsd_non_local_traffic", false)
	config.BindEnvAndSetDefault("dogstatsd_socket", GetPlatformDefault(map[string]interface{}{
		"linux": "/var/run/datadog/dsd.socket",
		"aix":   "/var/run/datadog/dsd.socket",
		"other": "",
	}))

	config.BindEnvAndSetDefault("dogstatsd_stream_socket", "")
	config.BindEnvAndSetDefault("dogstatsd_stream_log_too_big", false)
	config.BindEnvAndSetDefault("dogstatsd_pipeline_autoadjust", false)
	config.BindEnvAndSetDefault("dogstatsd_pipeline_count", 1)
	config.BindEnvAndSetDefault("dogstatsd_stats_port", 5000)
	config.BindEnvAndSetDefault("dogstatsd_stats_enable", false)
	config.BindEnvAndSetDefault("dogstatsd_stats_buffer", 10)
	config.BindEnvAndSetDefault("dogstatsd_telemetry_enabled_listener_id", false)
	// Control how dogstatsd-stats logs can be generated
	config.BindEnvAndSetDefault("dogstatsd_log_file", "${log_path}/dogstatsd_info/dogstatsd-stats.log")
	config.BindEnvAndSetDefault("dogstatsd_logging_enabled", true)
	config.BindEnvAndSetDefault("dogstatsd_log_file_max_rolls", 3)
	config.BindEnvAndSetDefault("dogstatsd_log_file_max_size", "10Mb")
	// Control for how long counter would be sampled to 0 if not received
	config.BindEnvAndSetDefault("dogstatsd_expiry_seconds", 300)
	config.BindEnvAndSetDefault("dogstatsd_flush_incomplete_buckets", false)
	// Control how long we keep dogstatsd contexts in memory.
	config.BindEnvAndSetDefault("dogstatsd_context_expiry_seconds", 20)
	// Only supported for socket traffic
	config.BindEnvAndSetDefault("dogstatsd_origin_detection", false)
	config.BindEnvAndSetDefault("dogstatsd_origin_detection_client", false)
	config.BindEnvAndSetDefault("dogstatsd_origin_optout_enabled", true)
	config.BindEnvAndSetDefault("dogstatsd_so_rcvbuf", 0)
	config.BindEnvAndSetDefault("dogstatsd_metrics_stats_enable", false)
	config.BindEnvAndSetDefault("dogstatsd_tags", []string{})
	config.BindEnvAndSetDefault("dogstatsd_mapper_cache_size", 1000)
	config.BindEnvAndSetDefault("dogstatsd_string_interner_size", 4096)
	// Enable check for Entity-ID presence when enriching Dogstatsd metrics with tags
	config.BindEnvAndSetDefault("dogstatsd_entity_id_precedence", false)
	// Sends Dogstatsd parse errors to the Debug level instead of the Error level
	config.BindEnvAndSetDefault("dogstatsd_disable_verbose_logs", false)
	// Location to store dogstatsd captures by default
	config.BindEnvAndSetDefault("dogstatsd_capture_path", "")
	// Depth of the channel the capture writer reads before persisting to disk.
	// Default is 0 - blocking channel
	config.BindEnvAndSetDefault("dogstatsd_capture_depth", 0)
	// Enable the no-aggregation pipeline.
	config.BindEnvAndSetDefault("dogstatsd_no_aggregation_pipeline", true)
	// How many metrics maximum in payloads sent by the no-aggregation pipeline to the intake.
	config.BindEnvAndSetDefault("dogstatsd_no_aggregation_pipeline_batch_size", 2048)
	// How many workers are used by the no-aggregation pipeline.
	config.BindEnvAndSetDefault("dogstatsd_no_aggregation_pipeline_workers_count", 1)
	// Force the amount of dogstatsd workers (mainly used for benchmarks or some very specific use-case)
	config.BindEnvAndSetDefault("dogstatsd_workers_count", 0)
	config.BindEnvAndSetDefault("dogstatsd_experimental_http.enabled", false)
	config.BindEnvAndSetDefault("dogstatsd_experimental_http.listen_address", "127.0.0.1:8125")

	// To enable the following feature, GODEBUG must contain `madvdontneed=1`
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.enabled", false)
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.low_soft_limit", float64(0.7))
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.high_soft_limit", float64(0.8))
	// 0 means don't call SetGCPercent
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.go_gc", 1)
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.memory_ballast", int64(1024*1024*1024*8))
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.rate_check.min", float64(0.01))
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.rate_check.max", 1)
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.rate_check.factor", 2)
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.soft_limit_freeos_check.min", float64(0.01))
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.soft_limit_freeos_check.max", float64(0.1))
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.soft_limit_freeos_check.factor", float64(1.5))

	config.BindEnvAndSetDefault("dogstatsd_mapper_profiles", []interface{}{})
	config.ParseEnvJSON("dogstatsd_mapper_profiles", []interface{}{})

	config.BindEnvAndSetDefault("statsd_forward_host", "")
	config.BindEnvAndSetDefault("statsd_forward_port", 0)
	config.BindEnvAndSetDefault("statsd_metric_namespace", "")
	config.BindEnvAndSetDefault("statsd_metric_namespace_blacklist", StandardStatsdPrefixes)

	config.BindEnvAndSetDefault("histogram_copy_to_distribution", false)
	config.BindEnvAndSetDefault("histogram_copy_to_distribution_prefix", "")
	config.BindEnvAndSetDefault("histogram_aggregates", []string{"max", "median", "avg", "count"})
	config.BindEnvAndSetDefault("histogram_percentiles", []string{"0.95"})
}

func logsagent(config pkgconfigmodel.Setup) {
	// External Use: modify those parameters to configure the logs-agent.
	// enable the logs-agent:
	config.BindEnvAndSetDefault("logs_enabled", false)
	// deprecated, use logs_enabled instead
	config.BindEnvAndSetDefault("log_enabled", false)
	// collect all logs from all containers:
	config.BindEnvAndSetDefault("logs_config.container_collect_all", false)
	// add a socks5 proxy:
	config.BindEnvAndSetDefault("logs_config.socks5_proxy_address", "")
	// disable distributed senders
	config.BindEnvAndSetDefault("logs_config.disable_distributed_senders", false)

	// DefaultFingerprintingCount refers to the number of lines or bytes to use for fingerprinting
	config.BindEnvAndSetDefault("logs_config.fingerprint_config.count", 0)

	// The maximum number of bytes that will be used to generate a checksum fingerprint;
	// used in cases where the line to hash is too large or if the fingerprinting maxLines=0
	config.BindEnvAndSetDefault("logs_config.fingerprint_config.max_bytes", 100000)

	// The default number of lines (or bytes) to skip when reading a file.
	// Whether we skip lines or bytes is dependent on whether we choose to compute the fingerprint by lines or by bytes.
	config.BindEnvAndSetDefault("logs_config.fingerprint_config.count_to_skip", 0)

	// DefaultFingerprintStrategy is the default strategy for computing the checksum fingerprint.
	// Options are:
	// - "line_checksum": compute the fingerprint by lines
	// - "byte_checksum": compute the fingerprint by bytes
	// - "disabled": disable fingerprinting
	config.BindEnvAndSetDefault("logs_config.fingerprint_config.fingerprint_strategy", "disabled")

	// specific logs-agent api-key
	config.BindEnvAndSetDefault("logs_config.api_key", "")
	// use the `time` field from container log files instead of ingestion time
	config.BindEnvAndSetDefault("logs_config.use_container_timestamp", false)
	// Delegated authentication for logs
	bindDelegatedAuthConfig(config, "logs_config")

	// Duration during which the host tags will be submitted with log events.
	// duration-formatted string (parsed by `time.ParseDuration`)
	config.BindEnvAndSetDefault("logs_config.expected_tags_duration", time.Duration(0))
	// send the logs to the port 443 of the logs-backend via TCP:
	config.BindEnvAndSetDefault("logs_config.use_port_443", false)
	// increase the read buffer size of the UDP sockets:
	config.BindEnvAndSetDefault("logs_config.frame_size", 9000)
	// maximum log message size in bytes
	config.BindEnvAndSetDefault("logs_config.max_message_size_bytes", DefaultMaxMessageSizeBytes)

	// Increase the number of files that can be tailed in parallel:
	// The default limit on darwin is 256. This is configurable per process on darwin with `ulimit -n` or a launchDaemon config.
	//
	// There is no effective limit for windows due to use of CreateFile win32 API
	// The OS default for most linux distributions is 1024
	config.BindEnvAndSetDefault("logs_config.open_files_limit",
		GetPlatformDefault(map[string]interface{}{
			"darwin": 200,
			"other":  500,
		}))
	// add global processing rules that are applied on all logs
	config.BindEnvAndSetDefault("logs_config.processing_rules", []interface{}{})
	// enforce the agent to use files to collect container logs on kubernetes environment
	config.BindEnvAndSetDefault("logs_config.k8s_container_use_file", false)
	// Tail a container's logs by querying the kubelet's API
	config.BindEnvAndSetDefault("logs_config.k8s_container_use_kubelet_api", false)
	// Enable the agent to use files to collect container logs on standalone docker environment, containers
	// with an existing registry offset will continue to be tailed from the docker socket unless
	// logs_config.docker_container_force_use_file is set to true.
	config.BindEnvAndSetDefault("logs_config.docker_container_use_file", true)
	// Force tailing from file for all docker container, even the ones with an existing registry entry
	config.BindEnvAndSetDefault("logs_config.docker_container_force_use_file", false)
	// While parsing Kubernetes pod logs, use /var/log/containers to validate that
	// the pod container ID is matching.
	config.BindEnvAndSetDefault("logs_config.validate_pod_container_id", true)
	// additional config to ensure initial logs are tagged with kubelet tags
	// wait (seconds) for tagger before start fetching tags of new AD services
	// Disabled by default (0 seconds)
	config.BindEnvAndSetDefault("logs_config.tagger_warmup_duration", 0)
	// Configurable docker client timeout while communicating with the docker daemon.
	// It could happen that the docker daemon takes a lot of time gathering timestamps
	// before starting to send any data when it has stored several large log files.
	// This field lets you increase the read timeout to prevent the client from
	// timing out too early in such a situation. Value in seconds.
	config.BindEnvAndSetDefault("logs_config.docker_client_read_timeout", 30)
	// Configurable API client timeout while communicating with the kubelet to stream logs. Value in seconds.
	config.BindEnvAndSetDefault("logs_config.kubelet_api_client_read_timeout", "30s")
	// Internal Use Only: avoid modifying those configuration parameters, this could lead to unexpected results.
	config.BindEnvAndSetDefault("logs_config.run_path", "${run_path}")
	// DEPRECATED in favor of `logs_config.force_use_http`.
	config.BindEnvAndSetDefault("logs_config.use_http", false)
	config.BindEnvAndSetDefault("logs_config.force_use_http", false)
	// DEPRECATED in favor of `logs_config.force_use_tcp`.
	config.BindEnvAndSetDefault("logs_config.use_tcp", false)
	config.BindEnvAndSetDefault("logs_config.force_use_tcp", false)
	// Maximum interval for HTTP connectivity retry checks with exponential backoff (in seconds)
	// When TCP fallback occurs, the agent will retry HTTP connectivity at increasing intervals
	// up to this ceiling, then continue checking at this interval. Default: 1 hour
	config.BindEnvAndSetDefault("logs_config.http_connectivity_retry_interval_max", "1h")

	// Transport protocol for log payloads
	config.BindEnvAndSetDefault("logs_config.http_protocol", "auto")
	config.BindEnvAndSetDefault("logs_config.http_timeout", 10)

	bindEnvAndSetLogsConfigKeys(config, "logs_config.")
	bindEnvAndSetLogsConfigKeys(config, "database_monitoring.samples.")
	bindEnvAndSetLogsConfigKeys(config, "database_monitoring.activity.")
	bindEnvAndSetLogsConfigKeys(config, "database_monitoring.metrics.")
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.aurora.enabled", false)
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.aurora.discovery_interval", 300)
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.aurora.region", "")
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.aurora.query_timeout", 10)
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.aurora.tags", []string{"datadoghq.com/scrape:true"})
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.aurora.dbm_tag", "datadoghq.com/dbm:true")
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.aurora.global_view_db_tag", "datadoghq.com/global_view_db")
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.rds.enabled", false)
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.rds.discovery_interval", 300)
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.rds.region", "")
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.rds.query_timeout", 10)
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.rds.tags", []string{"datadoghq.com/scrape:true"})
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.rds.dbm_tag", "datadoghq.com/dbm:true")
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.rds.global_view_db_tag", "datadoghq.com/global_view_db")

	bindEnvAndSetLogsConfigKeys(config, "data_streams.forwarder.")
	// 100ms for low-latency forwarding
	config.BindEnvAndSetDefault("data_streams.forwarder.batch_wait", float64(0.1))

	config.BindEnvAndSetDefault("logs_config.dd_port", 10516)
	config.BindEnvAndSetDefault("logs_config.dev_mode_use_proto", true)
	config.BindEnvAndSetDefault("logs_config.dd_url_443", "agent-443-intake.logs.datadoghq.com")
	config.BindEnvAndSetDefault("logs_config.stop_grace_period", 30)
	config.BindEnvAndSetDefault("logs_config.message_channel_size", 100)
	config.BindEnvAndSetDefault("logs_config.payload_channel_size", 10)

	// maximum time that the unix tailer will hold a log file open after it has been rotated
	config.BindEnvAndSetDefault("logs_config.close_timeout", 60)
	// maximum time that the windows tailer will hold a log file open, while waiting for
	// the downstream logs pipeline to be ready to accept more data
	config.BindEnvAndSetDefault("logs_config.windows_open_file_timeout", 5)

	// Auto multiline detection settings
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_detection", true)
	config.BindEnvAndSetDefault("logs_config.experimental_auto_multi_line_detection", false)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_detection_custom_samples", []map[string]interface{}{})
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.enable_json_detection", true)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.enable_datetime_detection", true)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.timestamp_detector_match_threshold", float64(0.5))
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.tokenizer_max_input_bytes", 60)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.pattern_table_max_size", 20)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.pattern_table_match_threshold", float64(0.75))
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.enable_json_aggregation", true)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.tag_aggregated_json", false)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.stack_trace_parsers", []string{"go"})

	// Adaptive sampler (experimental) rate-limits repetitive log patterns per source.
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.enabled", false)
	// Maximum number of distinct patterns the sampler tracks at once.
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.max_patterns", 1000)
	// Steady-state logs per second allowed for each matched pattern.
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.rate_limit", float64(1))
	// Maximum burst allowance per pattern, measured in accumulated credits/logs.
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.burst_size", float64(1000))
	// Fraction of tokens that must match for two logs to be treated as the same pattern.
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.match_threshold", float64(0.9))
	// The sampler needs a larger tokenizer window than the auto-multiline labeler.
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.tokenizer_max_input_bytes", 2048)
	// When true, logs containing critical severity keywords (FATAL, ERROR, PANIC, etc.)
	// bypass the adaptive sampler and are never dropped.
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.protect_important_logs", true)
	// When adaptive sampling or noisy log detection is enabled, tag logs with the hash of their structural sampler pattern.
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.tag_pattern_hash", false)
	// Include limits adaptive sampling to logs matching at least one rule when configured.
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.include", []map[string]interface{}{})
	// Exclude prevents adaptive sampling from applying to logs matching any rule when configured.
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.exclude", []map[string]interface{}{})
	// Sources listed here will not have adaptive sampling or noisy log detection applied, even if enabled globally.
	// Useful for auto-discovered sources that cannot be configured via integration-level YAML.
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.disabled_sources", []string{})
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.smart_severity_profiles.enabled", false)
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.smart_severity_profiles.cooldown", "5m")
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.smart_severity_profiles.medium.pass_through", false)
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.smart_severity_profiles.medium.rate_limit", float64(1))
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.smart_severity_profiles.medium.burst_size", float64(1000))
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.smart_severity_profiles.high.pass_through", false)
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.smart_severity_profiles.high.rate_limit", float64(1))
	config.BindEnvAndSetDefault("logs_config.experimental_adaptive_sampling.smart_severity_profiles.high.burst_size", float64(1000))
	// Tag repetitive logs that would be dropped by the adaptive sampler with noisy_log:true
	// without dropping them. Real adaptive sampling takes precedence when enabled.
	config.BindEnvAndSetDefault("logs_config.experimental_noisy_log_detection", false)

	// Enable the legacy auto multiline detection (v1)
	config.BindEnvAndSetDefault("logs_config.force_auto_multi_line_detection_v1", false)

	// The following auto_multi_line settings are settings for auto multiline detection v1
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_extra_patterns", []string{})
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_default_sample_size", 500)
	// Seconds
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_default_match_timeout", 30)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_default_match_threshold", float64(0.48))

	// Add a tag to logs that are multiline aggregated
	config.BindEnvAndSetDefault("logs_config.tag_multi_line_logs", true)
	// Add a tag to logs that are truncated by the agent
	config.BindEnvAndSetDefault("logs_config.tag_truncated_logs", true)
	// Tag logs with their auto multiline detection label without aggregating them
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_detection_tagging", true)

	// Number of logs pipeline instances. Defaults to number of logical CPU cores as defined by GOMAXPROCS or 4, whichever is lower.
	config.BindEnvAndSetDefault("logs_config.pipelines", 4)

	// If true, the agent looks for container logs in the location used by podman, rather
	// than docker.  This is a temporary configuration parameter to support podman logs until
	// a more substantial refactor of autodiscovery is made to determine this automatically.
	config.BindEnvAndSetDefault("logs_config.use_podman_logs", false)

	// If true, then a source_host tag (IP Address) will be added to TCP/UDP logs.
	config.BindEnvAndSetDefault("logs_config.use_sourcehost_tag", true)
	// Add per-line logsource:{stdout,stderr} tag to container logs (disabled by default)
	config.BindEnvAndSetDefault("logs_config.add_logsource_tag", false)

	// If set, the agent will look in this path for docker container log files.  Use this option if
	// docker's `data-root` has been set to a custom path and you wish to ingest docker logs from files. In
	// order to check your docker data-root directory, run the command `docker info -f '{{.DockerRootDir}}'`
	// See more documentation here:
	// https://docs.docker.com/engine/reference/commandline/dockerd/.
	config.BindEnvAndSetDefault("logs_config.docker_path_override", "")

	// Amount of time to wait for the container runtime to respond while determining what to log, containers or pods.
	// If the container runtime doesn't respond after specified timeout, log source marked as failed
	// which is reflected in the Agent status output for the Logs.
	// If this such behavior undesired, set the value to a significantly large number.
	// Timeout is in seconds.
	config.BindEnvAndSetDefault("logs_config.container_runtime_waiting_timeout", "3s")

	// in hours
	config.BindEnvAndSetDefault("logs_config.auditor_ttl", DefaultAuditorTTL)
	// Timeout in milliseonds used when performing agreggation operations,
	// including multi-line log processing rules and chunked line reaggregation.
	// It may be useful to increase it when logs writing is slowed down, that
	// could happen while serializing large objects on log lines.
	config.BindEnvAndSetDefault("logs_config.aggregation_timeout", 1000)
	// Time in seconds
	config.BindEnvAndSetDefault("logs_config.file_scan_period", float64(1))

	// Controls how wildcard file log source are prioritized when there are more files
	// that match wildcard log configurations than the `logs_config.open_files_limit`
	//
	// Choices are 'by_name' and 'by_modification_time'. See pkg/config/schema/yaml/ for full details.
	//
	// WARNING: 'by_modification_time' is less performant than 'by_name' and will trigger
	// more disk I/O at the wildcard log paths
	config.BindEnvAndSetDefault("logs_config.file_wildcard_selection_mode", "by_name")

	// Opt-in recursive glob for file log paths (supports **). Default false to preserve current behavior.
	config.BindEnvAndSetDefault("logs_config.enable_recursive_glob", false)

	// Max size in MB an integration logs file can use
	config.BindEnvAndSetDefault("logs_config.integrations_logs_files_max_size", 100)
	// Max disk usage in MB all integrations logs files are allowed to use in total
	config.BindEnvAndSetDefault("logs_config.integrations_logs_total_usage", 100)
	// Do not store logs on disk when the disk usage exceeds 80% of the disk capacity.
	config.BindEnvAndSetDefault("logs_config.integrations_logs_disk_ratio", float64(0.80))

	// Control how the stream-logs log file is managed
	config.BindEnvAndSetDefault("logs_config.streaming.streamlogs_log_file", "${log_path}/streamlogs_info/streamlogs.log")

	// If true, then the registry file will be written atomically. This behavior is not supported on ECS Fargate.
	config.BindEnvAndSetDefault("logs_config.atomic_registry_write",
		GetPlatformDefault(map[string]interface{}{
			"fargate": false,
			"other":   true,
		}),
	)

	// If true, exclude agent processes from process log collection
	config.BindEnvAndSetDefault("logs_config.process_exclude_agent", false)

	// Pipeline failover configuration
	config.BindEnvAndSetDefault("logs_config.pipeline_failover.enabled", false)
	config.BindEnvAndSetDefault("logs_config.pipeline_failover.router_channel_size", 5)
}

// vector integration
func vector(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("observability_pipelines_worker.metrics.enabled", false)
	config.BindEnvAndSetDefault("observability_pipelines_worker.metrics.url", "")
	config.BindEnvAndSetDefault("vector.metrics.enabled", false)
	config.BindEnvAndSetDefault("vector.metrics.url", "")
	config.BindEnvAndSetDefault("observability_pipelines_worker.metrics.use_v3_api.series", false)
	config.BindEnvAndSetDefault("vector.metrics.use_v3_api.series", false)
	config.BindEnvAndSetDefault("observability_pipelines_worker.logs.enabled", false)
	config.BindEnvAndSetDefault("observability_pipelines_worker.logs.url", "")
	config.BindEnvAndSetDefault("vector.logs.enabled", false)
	config.BindEnvAndSetDefault("vector.logs.url", "")

	// dual_ship is logs-only: there is no equivalent dual-shipping code path for metrics, so
	// these keys live outside bindVectorOptions to avoid registering an unused metrics variant.
	//
	// dual_ship: when false (default), OPW replaces the primary Datadog endpoint and is the only
	// destination logs are shipped to. When true, Datadog remains the primary endpoint and OPW is
	// added as an additional endpoint — intended for operators evaluating OPW without interrupting
	// the existing flow of telemetry to Datadog.
	//
	// dual_ship_reliable: when dual_ship=true, controls whether the OPW additional endpoint applies
	// backpressure to the main pipeline on failure (true) or is best-effort (false, the default).
	// Best-effort is the safer default: an unreachable OPW must not block delivery to Datadog.
	config.BindEnvAndSetDefault("observability_pipelines_worker.logs.dual_ship", false)
	config.BindEnvAndSetDefault("observability_pipelines_worker.logs.dual_ship_reliable", false)

	// Legacy vector.* aliases for dual_ship keys — users still on the legacy prefix must not have
	// dual_ship=true silently dropped when the fallback in obsPipelineWorkerDualShip reads these keys.
	config.BindEnvAndSetDefault("vector.logs.dual_ship", false)
	config.BindEnvAndSetDefault("vector.logs.dual_ship_reliable", false)
}

func cloudfoundry(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("cloud_foundry", false)
	config.BindEnvAndSetDefault("bosh_id", "")
	config.BindEnvAndSetDefault("cf_os_hostname_aliasing", false)
	config.BindEnvAndSetDefault("cloud_foundry_buildpack", false)
}

func containerd(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("containerd_namespace", []string{})
	// alias for containerd_namespace
	config.BindEnvAndSetDefault("containerd_namespaces", []string{})
	config.BindEnvAndSetDefault("containerd_exclude_namespaces", []string{"moby"})
	config.BindEnvAndSetDefault("container_env_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("container_labels_as_tags", map[string]string{})
}

func cri(config pkgconfigmodel.Setup) {
	// empty is disabled
	config.BindEnvAndSetDefault("cri_socket_path", "")
	// in seconds
	config.BindEnvAndSetDefault("cri_connection_timeout", int64(1))
	// in seconds
	config.BindEnvAndSetDefault("cri_query_timeout", int64(5))
}

func kubernetes(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("kubernetes_kubelet_host", "")
	config.BindEnvAndSetDefault("kubernetes_kubelet_nodename", "")
	config.BindEnvAndSetDefault("eks_fargate", false)
	config.BindEnvAndSetDefault("kubelet_use_api_server", false)
	config.BindEnvAndSetDefault("kubernetes_http_kubelet_port", 10255)
	config.BindEnvAndSetDefault("kubernetes_https_kubelet_port", 10250)

	config.BindEnvAndSetDefault("kubelet_tls_verify", true)
	config.BindEnvAndSetDefault("kubelet_core_check_enabled", true)
	config.BindEnvAndSetDefault("collect_kubernetes_events", false)
	config.BindEnvAndSetDefault("kubernetes_events_source_detection.enabled", false)
	config.BindEnvAndSetDefault("kubelet_client_ca", "")

	config.BindEnvAndSetDefault("kubelet_auth_token_path", "")
	config.BindEnvAndSetDefault("kubelet_client_crt", "")
	config.BindEnvAndSetDefault("kubelet_client_key", "")

	// in seconds, default 15 minutes
	config.BindEnvAndSetDefault("kubernetes_pod_expiration_duration", 15*60)
	// Cache TTL for pod lists in the kubelet client. Set to 0 because it's only
	// used by workloadmeta that already defines its own pull frequency and has
	// its own storage, so no need for an extra cache.
	config.BindEnvAndSetDefault("kubelet_cache_pods_duration", 0)
	// not exposed yet. In seconds. 0 means use workloadmeta default
	config.BindEnvAndSetDefault("kubelet_collector_pull_interval", 0)
	config.BindEnvAndSetDefault("kubernetes_collect_metadata_tags", true)
	config.BindEnvAndSetDefault("kubernetes_use_endpoint_slices", false)
	// Polling frequency of the Agent to the DCA in seconds (gets the local cache if the DCA is disabled)
	config.BindEnvAndSetDefault("kubernetes_metadata_tag_update_freq", 60)
	config.BindEnvAndSetDefault("kubernetes_metadata_streaming", true)
	config.BindEnvAndSetDefault("kubernetes_apiserver_client_timeout", 10)
	config.BindEnvAndSetDefault("kubernetes_apiserver_informer_client_timeout", 0)
	config.BindEnvAndSetDefault("kubernetes_apiserver_client_qps", 40)
	config.BindEnvAndSetDefault("kubernetes_apiserver_client_burst", 200)
	// temporary opt-out of the new mapping logic
	config.BindEnvAndSetDefault("kubernetes_map_services_on_ip", false)
	config.BindEnvAndSetDefault("kubernetes_apiserver_use_protobuf", false)
	config.BindEnvAndSetDefault("kubernetes_ad_tags_disabled", []string{})
	config.BindEnvAndSetDefault("kubernetes_kube_service_ignore_readiness", false)

	config.BindEnvAndSetDefault("kubernetes_kubelet_podresources_socket", GetPlatformDefault(map[string]interface{}{
		"windows": `\\.\pipe\kubelet-pod-resources`,
		"other":   "/var/lib/kubelet/pod-resources/kubelet.sock",
	}))
	config.BindEnvAndSetDefault("kubernetes_kubelet_deviceplugins_socketdir", GetPlatformDefault(map[string]interface{}{
		"windows": `\\.\pipe\kubelet-device-plugins`,
		"other":   "/var/lib/kubelet/device-plugins",
	}))

	config.BindEnvAndSetDefault("kubernetes_kubelet_deviceplugins_cache_duration", 5*time.Second)
}

func podman(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("podman_db_path", "")
}

func anomalyDetection(config pkgconfigmodel.Setup) {
	// Log ingestion gate. When false, all log sources (container, kubelet, internal)
	// are not routed into the anomaly detection pipeline (recording is unaffected).
	config.BindEnvAndSetDefault("anomaly_detection.logs.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.logs.containers.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.logs.kubelet.enabled", true)

	// Internal agent log tap.
	// min_severity is the minimum level forwarded (logs below it are dropped before sampling).
	// max_rate_* are in logs/second measured over a 10-second fixed window and apply
	// after the min_severity gate. -1 means unlimited (no cap); 0 drops all logs of that priority.
	// high_priority = warn/error/critical, medium_priority = info, low_priority = debug.
	config.BindEnvAndSetDefault("anomaly_detection.logs.internal.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.logs.internal.min_severity", "warn")
	// unlimited
	config.BindEnvAndSetDefault("anomaly_detection.logs.internal.max_rate_high_priority", float64(-1))
	config.BindEnvAndSetDefault("anomaly_detection.logs.internal.max_rate_medium_priority", float64(100))
	config.BindEnvAndSetDefault("anomaly_detection.logs.internal.max_rate_low_priority", float64(1))

	// Kubelet journald log rate limits (enabled flag already set above).
	config.BindEnvAndSetDefault("anomaly_detection.logs.kubelet.min_severity", "warn")
	// unlimited
	config.BindEnvAndSetDefault("anomaly_detection.logs.kubelet.max_rate_high_priority", float64(-1))
	config.BindEnvAndSetDefault("anomaly_detection.logs.kubelet.max_rate_medium_priority", float64(100))
	config.BindEnvAndSetDefault("anomaly_detection.logs.kubelet.max_rate_low_priority", float64(1))

	// Container log rate limits (enabled flag already set above).
	config.BindEnvAndSetDefault("anomaly_detection.logs.containers.min_severity", "warn")
	// unlimited
	config.BindEnvAndSetDefault("anomaly_detection.logs.containers.max_rate_high_priority", float64(-1))
	config.BindEnvAndSetDefault("anomaly_detection.logs.containers.max_rate_medium_priority", float64(100))
	config.BindEnvAndSetDefault("anomaly_detection.logs.containers.max_rate_low_priority", float64(1))

	// Metrics ingestion gate. When false, externally-ingested metrics
	// (DogStatsD, check samplers) are dropped at the handle factory.
	// Log-derived virtual metrics are unaffected.
	config.BindEnvAndSetDefault("anomaly_detection.metrics.enabled", true)

	// Stdout reporting verbosity.
	// stdout.enabled: set to false to silence all [observer] stdout log lines.
	// stdout.verbose: set to true to print individual anomaly series after the title line.
	// Default: title-only (stdout.enabled=true, stdout.verbose=false).
	config.BindEnvAndSetDefault("anomaly_detection.reporting.stdout.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.reporting.stdout.verbose", false)

	// Datadog event reporting. Keep false during evaluation / shadow mode.
	config.BindEnvAndSetDefault("anomaly_detection.reporting.events.enabled", false)

	// Parquet recording of raw ingested signals for offline analysis and replay.
	config.BindEnvAndSetDefault("anomaly_detection.recording.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.recording.output_dir", "/var/run/datadog/anomaly_detection")
	config.BindEnvAndSetDefault("anomaly_detection.recording.flush_interval", 60)
	config.BindEnvAndSetDefault("anomaly_detection.recording.retention", "24h")

	// Debug tooling: internal state dump. Leave empty/zero in production.
	config.BindEnvAndSetDefault("anomaly_detection.debug.dump_path", "")
	config.BindEnvAndSetDefault("anomaly_detection.debug.dump_interval", 0)
	config.BindEnvAndSetDefault("anomaly_detection.debug.events_dump_path", "")

	// Ordered metric-processing rules applied at the observer handle boundary.
	config.BindEnvAndSetDefault("anomaly_detection.metrics.processing_rules", []map[string]interface{}{})

	// Ordered log-processing rules applied to anomaly-detection log sources.
	config.BindEnvAndSetDefault("anomaly_detection.logs.processing_rules", []map[string]interface{}{})

	// Detector/correlator/extractor toggles. Defaults match componentCatalog.defaultEnabled.
	config.BindEnvAndSetDefault("anomaly_detection.detectors.log_metrics_extractor.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.connection_error_extractor.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.log_pattern_extractor.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.cusum.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.bocpd.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.rrcf.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.scanmw.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.scanwelch.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.holt_residual.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.tukey_biweight.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.cross_signal.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.time_cluster.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.time_cluster.min_cluster_size", 0)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.passthrough.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.anomaly_scorer.dry_run.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.anomaly_scorer.alpha", float64(0.014))
	config.BindEnvAndSetDefault("anomaly_detection.anomaly_scorer.saturation_k", float64(5))
	config.BindEnvAndSetDefault("anomaly_detection.anomaly_scorer.window", 15*time.Second)
	config.BindEnvAndSetDefault("anomaly_detection.anomaly_scorer.low_threshold", float64(0.15))
	config.BindEnvAndSetDefault("anomaly_detection.anomaly_scorer.high_threshold", float64(0.40))
	config.BindEnvAndSetDefault("anomaly_detection.anomaly_scorer.margin_pct", float64(0.20))
	config.BindEnvAndSetDefault("anomaly_detection.anomaly_scorer.output.correlation_events", false)
	config.BindEnvAndSetDefault("anomaly_detection.anomaly_scorer.output.correlation_event_threshold", "high")
	config.BindEnvAndSetDefault("anomaly_detection.anomaly_scorer.output.logs", false)
	config.BindEnvAndSetDefault("anomaly_detection.anomaly_scorer.output.cooldown", 300*time.Second)
	config.BindEnvAndSetDefault("anomaly_detection.anomaly_scorer.output.max_anomalies", 50)

	// Storage tuning. See storageConfig in the observer component.
	config.BindEnvAndSetDefault("anomaly_detection.storage.max_series", 50000)
	config.BindEnvAndSetDefault("anomaly_detection.storage.eviction_floor_ratio", float64(0.5))
	config.BindEnvAndSetDefault("anomaly_detection.storage.point_retention", 120*time.Second)

	// Baseline analysis window.
	config.BindEnvAndSetDefault("anomaly_detection.baseline_analysis.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.baseline_analysis.mute_noisy_metrics", true)
	config.BindEnvAndSetDefault("anomaly_detection.baseline_analysis.duration", "10m")
	config.BindEnvAndSetDefault("anomaly_detection.baseline_analysis.verbose", false)
}
