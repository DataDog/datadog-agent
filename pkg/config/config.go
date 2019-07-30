// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// DefaultForwarderRecoveryInterval is the default recovery interval, also used if
// the user-provided value is invalid.
const DefaultForwarderRecoveryInterval = 2

const megaByte = 1024 * 1024

// DefaultSite is the default site the Agent sends data to.
const DefaultSite = "datadoghq.com"

const infraURLPrefix = "https://app."

var overrideVars = map[string]interface{}{}

// Datadog is the global configuration object
var (
	Datadog Config
	proxies *Proxy
)

// MetadataProviders helps unmarshalling `metadata_providers` config param
type MetadataProviders struct {
	Name     string        `mapstructure:"name"`
	Interval time.Duration `mapstructure:"interval"`
}

// ConfigurationProviders helps unmarshalling `config_providers` config param
type ConfigurationProviders struct {
	Name             string `mapstructure:"name"`
	Polling          bool   `mapstructure:"polling"`
	PollInterval     string `mapstructure:"poll_interval"`
	TemplateURL      string `mapstructure:"template_url"`
	TemplateDir      string `mapstructure:"template_dir"`
	Username         string `mapstructure:"username"`
	Password         string `mapstructure:"password"`
	CAFile           string `mapstructure:"ca_file"`
	CAPath           string `mapstructure:"ca_path"`
	CertFile         string `mapstructure:"cert_file"`
	KeyFile          string `mapstructure:"key_file"`
	Token            string `mapstructure:"token"`
	GraceTimeSeconds int    `mapstructure:"grace_time_seconds"`
}

// Listeners helps unmarshalling `listeners` config param
type Listeners struct {
	Name string `mapstructure:"name"`
}

// Proxy represents the configuration for proxies in the agent
type Proxy struct {
	HTTP    string   `mapstructure:"http"`
	HTTPS   string   `mapstructure:"https"`
	NoProxy []string `mapstructure:"no_proxy"`
}

func init() {
	osinit()
	// Configure Datadog global configuration
	Datadog = NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	// Configuration defaults
	initConfig(Datadog)
}

// initConfig initializes the config defaults on a config
func initConfig(config Config) {
	// Agent
	// Don't set a default on 'site' to allow detecting with viper whether it's set in config
	config.BindEnv("site")
	config.BindEnv("dd_url")
	config.BindEnvAndSetDefault("app_key", "")
	config.SetDefault("proxy", nil)
	config.BindEnvAndSetDefault("skip_ssl_validation", false)
	config.BindEnvAndSetDefault("hostname", "")
	config.BindEnvAndSetDefault("tags", []string{})
	config.BindEnvAndSetDefault("tag_value_split_separator", map[string]string{})
	config.BindEnvAndSetDefault("conf_path", ".")
	config.BindEnvAndSetDefault("confd_path", defaultConfdPath)
	config.BindEnvAndSetDefault("additional_checksd", defaultAdditionalChecksPath)
	config.BindEnvAndSetDefault("log_payloads", false)
	config.BindEnvAndSetDefault("log_file", "")
	config.BindEnvAndSetDefault("log_file_max_size", "10Mb")
	config.BindEnvAndSetDefault("log_file_max_rolls", 1)
	config.BindEnvAndSetDefault("log_level", "info")
	config.BindEnvAndSetDefault("log_to_syslog", false)
	config.BindEnvAndSetDefault("log_to_console", true)
	config.BindEnvAndSetDefault("logging_frequency", int64(500))
	config.BindEnvAndSetDefault("disable_file_logging", false)
	config.BindEnvAndSetDefault("syslog_uri", "")
	config.BindEnvAndSetDefault("syslog_rfc", false)
	config.BindEnvAndSetDefault("syslog_pem", "")
	config.BindEnvAndSetDefault("syslog_key", "")
	config.BindEnvAndSetDefault("syslog_tls_verify", true)
	config.BindEnvAndSetDefault("cmd_host", "localhost")
	config.BindEnvAndSetDefault("cmd_port", 5001)
	config.BindEnvAndSetDefault("cluster_agent.cmd_port", 5005)
	config.BindEnvAndSetDefault("default_integration_http_timeout", 9)
	config.BindEnvAndSetDefault("enable_metadata_collection", true)
	config.BindEnvAndSetDefault("enable_gohai", true)
	config.BindEnvAndSetDefault("check_runners", int64(4))
	config.BindEnvAndSetDefault("auth_token_file_path", "")
	config.BindEnvAndSetDefault("bind_host", "localhost")
	config.BindEnvAndSetDefault("health_port", int64(0))
	config.BindEnvAndSetDefault("disable_py3_validation", false)
	config.BindEnvAndSetDefault("python_version", "2")
	// C-land crash feature flags
	config.BindEnvAndSetDefault("c_stacktrace_collection", false)
	config.BindEnvAndSetDefault("c_core_dump", false)

	// if/when the default is changed to true, make the default platform
	// dependent; default should remain false on Windows to maintain backward
	// compatibility with Agent5 behavior/win
	config.BindEnvAndSetDefault("hostname_fqdn", false)
	config.BindEnvAndSetDefault("cluster_name", "")

	// secrets backend
	config.BindEnvAndSetDefault("secret_backend_command", "")
	config.BindEnvAndSetDefault("secret_backend_arguments", []string{})
	config.BindEnvAndSetDefault("secret_backend_output_max_size", 1024)
	config.BindEnvAndSetDefault("secret_backend_timeout", 5)

	// Retry settings
	config.BindEnvAndSetDefault("forwarder_backoff_factor", 2)
	config.BindEnvAndSetDefault("forwarder_backoff_base", 2)
	config.BindEnvAndSetDefault("forwarder_backoff_max", 64)
	config.BindEnvAndSetDefault("forwarder_recovery_interval", DefaultForwarderRecoveryInterval)
	config.BindEnvAndSetDefault("forwarder_recovery_reset", false)

	// Use to output logs in JSON format
	config.BindEnvAndSetDefault("log_format_json", false)

	// IPC API server timeout
	config.BindEnvAndSetDefault("server_timeout", 15)

	// Use to force client side TLS version to 1.2
	config.BindEnvAndSetDefault("force_tls_12", false)

	// Defaults to safe YAML methods in base and custom checks.
	config.BindEnvAndSetDefault("disable_unsafe_yaml", true)

	// Agent GUI access port
	config.BindEnvAndSetDefault("GUI_port", defaultGuiPort)
	if IsContainerized() {
		config.SetDefault("procfs_path", "/host/proc")
		config.SetDefault("container_proc_root", "/host/proc")
		config.SetDefault("container_cgroup_root", "/host/sys/fs/cgroup/")
	} else {
		config.SetDefault("container_proc_root", "/proc")
		// for amazon linux the cgroup directory on host is /cgroup/
		// we pick memory.stat to make sure it exists and not empty
		if _, err := os.Stat("/cgroup/memory/memory.stat"); !os.IsNotExist(err) {
			config.SetDefault("container_cgroup_root", "/cgroup/")
		} else {
			config.SetDefault("container_cgroup_root", "/sys/fs/cgroup/")
		}
	}

	config.BindEnv("procfs_path")
	config.BindEnv("container_proc_root")
	config.BindEnv("container_cgroup_root")

	config.BindEnvAndSetDefault("proc_root", "/proc")
	config.BindEnvAndSetDefault("histogram_aggregates", []string{"max", "median", "avg", "count"})
	config.BindEnvAndSetDefault("histogram_percentiles", []string{"0.95"})
	// Serializer
	config.BindEnvAndSetDefault("enable_stream_payload_serialization", true)
	// Warning: do not change the two following values. Your payloads will get dropped by Datadog's intake.
	config.BindEnvAndSetDefault("serializer_max_payload_size", 2*megaByte+megaByte/2)
	config.BindEnvAndSetDefault("serializer_max_uncompressed_payload_size", 4*megaByte)
	config.BindEnvAndSetDefault("use_v2_api.series", false)
	config.BindEnvAndSetDefault("use_v2_api.events", false)
	config.BindEnvAndSetDefault("use_v2_api.service_checks", false)
	// Serializer: allow user to blacklist any kind of payload to be sent
	config.BindEnvAndSetDefault("enable_payloads.events", true)
	config.BindEnvAndSetDefault("enable_payloads.series", true)
	config.BindEnvAndSetDefault("enable_payloads.service_checks", true)
	config.BindEnvAndSetDefault("enable_payloads.sketches", true)
	config.BindEnvAndSetDefault("enable_payloads.json_to_v1_intake", true)

	// Forwarder
	config.BindEnvAndSetDefault("forwarder_timeout", 20)
	config.BindEnvAndSetDefault("forwarder_retry_queue_max_size", 30)
	config.BindEnvAndSetDefault("forwarder_num_workers", 1)
	// Dogstatsd
	config.BindEnvAndSetDefault("use_dogstatsd", true)
	config.BindEnvAndSetDefault("dogstatsd_port", 8125) // Notice: 0 means UDP port closed

	// The following options allow to configure how the dogstatsd intake buffers and queues incoming datagrams.
	// When a datagram is received it is first added to a datagrams buffer. This buffer fills up until
	// we reach `dogstatsd_packet_buffer_size` datagrams or after `dogstatsd_packet_buffer_flush_timeout` ms.
	// After this happens we flush this buffer of datagrams to a queue for processing. The size of this queue
	// is `dogstatsd_queue_size`.
	config.BindEnvAndSetDefault("dogstatsd_buffer_size", 1024*8)
	config.BindEnvAndSetDefault("dogstatsd_packet_buffer_size", 512)
	config.BindEnvAndSetDefault("dogstatsd_packet_buffer_flush_timeout", 100*time.Millisecond)
	config.BindEnvAndSetDefault("dogstatsd_queue_size", 100)

	config.BindEnvAndSetDefault("dogstatsd_non_local_traffic", false)
	config.BindEnvAndSetDefault("dogstatsd_socket", "") // Notice: empty means feature disabled
	config.BindEnvAndSetDefault("dogstatsd_stats_port", 5000)
	config.BindEnvAndSetDefault("dogstatsd_stats_enable", false)
	config.BindEnvAndSetDefault("dogstatsd_stats_buffer", 10)
	config.BindEnvAndSetDefault("dogstatsd_expiry_seconds", 300)
	config.BindEnvAndSetDefault("dogstatsd_origin_detection", false) // Only supported for socket traffic
	config.BindEnvAndSetDefault("dogstatsd_so_rcvbuf", 0)
	config.BindEnvAndSetDefault("dogstatsd_metrics_stats_enable", false)
	config.BindEnvAndSetDefault("dogstatsd_tags", []string{})
	config.BindEnvAndSetDefault("statsd_forward_host", "")
	config.BindEnvAndSetDefault("statsd_forward_port", 0)
	config.BindEnvAndSetDefault("statsd_metric_namespace", "")
	config.BindEnvAndSetDefault("statsd_metric_namespace_blacklist", StandardStatsdPrefixes)
	// Autoconfig
	config.BindEnvAndSetDefault("autoconf_template_dir", "/datadog/check_configs")
	config.BindEnvAndSetDefault("exclude_pause_container", true)
	config.BindEnvAndSetDefault("ac_include", []string{})
	config.BindEnvAndSetDefault("ac_exclude", []string{})
	config.BindEnvAndSetDefault("ad_config_poll_interval", int64(10)) // in seconds
	config.BindEnvAndSetDefault("extra_listeners", []string{})
	config.BindEnvAndSetDefault("extra_config_providers", []string{})

	// Docker
	config.BindEnvAndSetDefault("docker_query_timeout", int64(5))
	config.BindEnvAndSetDefault("docker_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("docker_env_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_pod_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_pod_annotations_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_node_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("container_cgroup_prefix", "")

	// CRI
	config.BindEnvAndSetDefault("cri_socket_path", "")              // empty is disabled
	config.BindEnvAndSetDefault("cri_connection_timeout", int64(1)) // in seconds
	config.BindEnvAndSetDefault("cri_query_timeout", int64(5))      // in seconds

	// Containerd
	// We only support containerd in Kubernetes. By default containerd cri uses `k8s.io` https://github.com/containerd/cri/blob/release/1.2/pkg/constants/constants.go#L22-L23
	config.BindEnvAndSetDefault("containerd_namespace", "k8s.io")

	// Kubernetes
	config.BindEnvAndSetDefault("kubernetes_kubelet_host", "")
	config.BindEnvAndSetDefault("kubernetes_http_kubelet_port", 10255)
	config.BindEnvAndSetDefault("kubernetes_https_kubelet_port", 10250)

	config.BindEnvAndSetDefault("kubelet_tls_verify", true)
	config.BindEnvAndSetDefault("collect_kubernetes_events", false)
	config.BindEnvAndSetDefault("kubelet_client_ca", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")

	config.BindEnvAndSetDefault("kubelet_auth_token_path", "")
	config.BindEnvAndSetDefault("kubelet_client_crt", "")
	config.BindEnvAndSetDefault("kubelet_client_key", "")

	config.BindEnvAndSetDefault("kubernetes_pod_expiration_duration", 15*60) // in seconds, default 15 minutes
	config.BindEnvAndSetDefault("kubelet_wait_on_missing_container", 0)
	config.BindEnvAndSetDefault("kubelet_cache_pods_duration", 5)       // Polling frequency in seconds of the agent to the kubelet "/pods" endpoint
	config.BindEnvAndSetDefault("kubelet_listener_polling_interval", 5) // Polling frequency in seconds of the pod watcher to detect new pods/containers (affected by kubelet_cache_pods_duration setting)
	config.BindEnvAndSetDefault("kubernetes_collect_metadata_tags", true)
	config.BindEnvAndSetDefault("kubernetes_metadata_tag_update_freq", 60) // Polling frequency of the Agent to the DCA in seconds (gets the local cache if the DCA is disabled)
	config.BindEnvAndSetDefault("kubernetes_apiserver_client_timeout", 10)
	config.BindEnvAndSetDefault("kubernetes_map_services_on_ip", false) // temporary opt-out of the new mapping logic
	config.BindEnvAndSetDefault("kubernetes_apiserver_use_protobuf", false)

	// Kube ApiServer
	config.BindEnvAndSetDefault("kubernetes_kubeconfig_path", "")
	config.BindEnvAndSetDefault("leader_lease_duration", "60")
	config.BindEnvAndSetDefault("leader_election", false)
	config.BindEnvAndSetDefault("kube_resources_namespace", "")

	// Datadog cluster agent
	config.BindEnvAndSetDefault("cluster_agent.enabled", false)
	config.BindEnvAndSetDefault("cluster_agent.auth_token", "")
	config.BindEnvAndSetDefault("cluster_agent.url", "")
	config.BindEnvAndSetDefault("cluster_agent.kubernetes_service_name", "datadog-cluster-agent")
	config.BindEnvAndSetDefault("metrics_port", "5000")

	// Metadata endpoints

	// Defines the maximum size of hostame gathered from EC2, GCE, Azure and Alibabacloud metadata endpoints.
	// Used internally to protect against configurations where metadata endpoints return incorrect values with 200 status codes.
	config.BindEnvAndSetDefault("metadata_endpoints_max_hostname_size", 255)

	// ECS
	config.BindEnvAndSetDefault("ecs_agent_url", "") // Will be autodetected
	config.BindEnvAndSetDefault("ecs_agent_container_name", "ecs-agent")
	config.BindEnvAndSetDefault("collect_ec2_tags", false)

	// GCE
	config.BindEnvAndSetDefault("collect_gce_tags", true)

	// Cloud Foundry
	config.BindEnvAndSetDefault("cloud_foundry", false)
	config.BindEnvAndSetDefault("bosh_id", "")
	config.BindEnvAndSetDefault("cf_os_hostname_aliasing", false)

	// JMXFetch
	config.BindEnvAndSetDefault("jmx_custom_jars", []string{})
	config.BindEnvAndSetDefault("jmx_use_cgroup_memory_limit", false)
	config.BindEnvAndSetDefault("jmx_max_restarts", int64(3))
	config.BindEnvAndSetDefault("jmx_restart_interval", int64(5))
	config.BindEnvAndSetDefault("jmx_thread_pool_size", 3)
	config.BindEnvAndSetDefault("jmx_reconnection_thread_pool_size", 3)
	config.BindEnvAndSetDefault("jmx_collection_timeout", 60)
	config.BindEnvAndSetDefault("jmx_check_period", int(defaults.DefaultCheckInterval/time.Millisecond))
	config.BindEnvAndSetDefault("jmx_reconnection_timeout", 10)

	// Go_expvar server port
	config.BindEnvAndSetDefault("expvar_port", "5000")

	// Trace agent
	config.BindEnvAndSetDefault("apm_config.enabled", true)

	// Process agent
	config.SetDefault("process_config.enabled", "false")
	config.BindEnv("process_config.process_dd_url", "")

	// Logs Agent

	// External Use: modify those parameters to configure the logs-agent.
	// enable the logs-agent:
	config.BindEnvAndSetDefault("logs_enabled", false)
	config.BindEnvAndSetDefault("log_enabled", false) // deprecated, use logs_enabled instead
	// collect all logs from all containers:
	config.BindEnvAndSetDefault("logs_config.container_collect_all", false)
	// add a socks5 proxy:
	config.BindEnvAndSetDefault("logs_config.socks5_proxy_address", "")
	// send the logs to a proxy:
	config.BindEnv("logs_config.logs_dd_url") // must respect format '<HOST>:<PORT>' and '<PORT>' to be an integer
	// specific logs-agent api-key
	config.BindEnv("logs_config.api_key")
	config.BindEnvAndSetDefault("logs_config.logs_no_ssl", false)
	// send the logs to the port 443 of the logs-backend via TCP:
	config.BindEnvAndSetDefault("logs_config.use_port_443", false)
	// increase the read buffer size of the UDP sockets:
	config.BindEnvAndSetDefault("logs_config.frame_size", 9000)
	// increase the number of files that can be tailed in parallel:
	config.BindEnvAndSetDefault("logs_config.open_files_limit", 100)
	// add global processing rules that are applied on all logs
	config.BindEnv("logs_config.processing_rules")
	// enforce the agent to use files to collect container logs on kubernetes environment
	config.BindEnvAndSetDefault("logs_config.k8s_container_use_file", false)

	// Internal Use Only: avoid modifying those configuration parameters, this could lead to unexpected results.
	config.BindEnvAndSetDefault("logs_config.run_path", defaultRunPath)
	config.BindEnv("logs_config.dd_url")
	config.BindEnvAndSetDefault("logs_config.use_http", false)
	config.BindEnvAndSetDefault("logs_config.dd_port", 10516)
	config.BindEnvAndSetDefault("logs_config.dev_mode_use_proto", true)
	config.BindEnvAndSetDefault("logs_config.dd_url_443", "agent-443-intake.logs.datadoghq.com")
	config.BindEnvAndSetDefault("logs_config.stop_grace_period", 30)
	config.SetKnown("logs_config.additional_endpoints")

	// The cardinality of tags to send for checks and dogstatsd respectively.
	// Choices are: low, orchestrator, high.
	// WARNING: sending orchestrator, or high tags for dogstatsd metrics may create more metrics
	// (one per container instead of one per host).
	// Changing this setting may impact your custom metrics billing.
	config.BindEnvAndSetDefault("checks_tag_cardinality", "low")
	config.BindEnvAndSetDefault("dogstatsd_tag_cardinality", "low")

	config.BindEnvAndSetDefault("histogram_copy_to_distribution", false)
	config.BindEnvAndSetDefault("histogram_copy_to_distribution_prefix", "")

	config.BindEnv("api_key")

	config.BindEnvAndSetDefault("hpa_watcher_polling_freq", 10)
	config.BindEnvAndSetDefault("hpa_watcher_gc_period", 60*5) // 5 minutes
	config.BindEnvAndSetDefault("external_metrics_provider.enabled", false)
	config.BindEnvAndSetDefault("external_metrics_provider.port", 443)
	config.BindEnvAndSetDefault("hpa_configmap_name", "datadog-custom-metrics")
	config.BindEnvAndSetDefault("external_metrics_provider.refresh_period", 30)          // value in seconds. Frequency of batch calls to the ConfigMap persistent store (GlobalStore) by the Leader.
	config.BindEnvAndSetDefault("external_metrics_provider.batch_window", 10)            // value in seconds. Batch the events from the Autoscalers informer to push updates to the ConfigMap (GlobalStore)
	config.BindEnvAndSetDefault("external_metrics_provider.max_age", 120)                // value in seconds. 4 cycles from the HPA controller (up to Kubernetes 1.11) is enough to consider a metric stale
	config.BindEnvAndSetDefault("external_metrics.aggregator", "avg")                    // aggregator used for the external metrics. Choose from [avg,sum,max,min]
	config.BindEnvAndSetDefault("external_metrics_provider.bucket_size", 60*5)           // Window to query to get the metric from Datadog.
	config.BindEnvAndSetDefault("external_metrics_provider.rollup", 30)                  // Bucket size to circumvent time aggregation side effects.
	config.BindEnvAndSetDefault("kubernetes_event_collection_timeout", 100)              // timeout between two successful event collections in milliseconds.
	config.BindEnvAndSetDefault("kubernetes_informers_resync_period", 60*5)              // value in seconds. Default to 5 minutes
	config.BindEnvAndSetDefault("external_metrics_provider.local_copy_refresh_rate", 30) // value in seconds
	// Cluster check Autodiscovery
	config.BindEnvAndSetDefault("cluster_checks.enabled", false)
	config.BindEnvAndSetDefault("cluster_checks.node_expiration_timeout", 30) // value in seconds
	config.BindEnvAndSetDefault("cluster_checks.warmup_duration", 30)         // value in seconds
	config.BindEnvAndSetDefault("cluster_checks.cluster_tag_name", "cluster_name")
	config.BindEnvAndSetDefault("cluster_checks.extra_tags", []string{})

	// Declare other keys that don't have a default/env var.
	// Mostly, keys we use IsSet() on, because IsSet always returns true if a key has a default.
	config.SetKnown("metadata_providers")
	config.SetKnown("config_providers")
	config.SetKnown("clustername")
	config.SetKnown("listeners")
	config.SetKnown("additional_endpoints.*")
	config.SetKnown("proxy.http")
	config.SetKnown("proxy.https")
	config.SetKnown("proxy.no_proxy")

	// Process agent
	config.SetKnown("process_config.dd_agent_env")
	config.SetKnown("process_config.enabled")
	config.SetKnown("process_config.intervals.process_realtime")
	config.SetKnown("process_config.queue_size")
	config.SetKnown("process_config.max_per_message")
	config.SetKnown("process_config.intervals.process")
	config.SetKnown("process_config.blacklist_patterns")
	config.SetKnown("process_config.intervals.container")
	config.SetKnown("process_config.intervals.container_realtime")
	config.SetKnown("process_config.dd_agent_bin")
	config.SetKnown("process_config.custom_sensitive_words")
	config.SetKnown("process_config.scrub_args")
	config.SetKnown("process_config.strip_proc_arguments")
	config.SetKnown("process_config.windows.args_refresh_interval")
	config.SetKnown("process_config.windows.add_new_args")
	config.SetKnown("process_config.additional_endpoints.*")
	config.SetKnown("process_config.container_source")
	config.SetKnown("process_config.intervals.connections")
	config.SetKnown("process_config.expvar_port")

	// System probe
	config.SetKnown("system_probe_config.enabled")
	config.SetKnown("system_probe_config.log_file")
	config.SetKnown("system_probe_config.debug_port")
	config.SetKnown("system_probe_config.bpf_debug")
	config.SetKnown("system_probe_config.disable_tcp")
	config.SetKnown("system_probe_config.disable_udp")
	config.SetKnown("system_probe_config.disable_ipv6")
	config.SetKnown("system_probe_config.collect_local_dns")
	config.SetKnown("system_probe_config.use_local_system_probe")
	config.SetKnown("system_probe_config.enable_conntrack")
	config.SetKnown("system_probe_config.sysprobe_socket")
	config.SetKnown("system_probe_config.conntrack_short_term_buffer_size")
	config.SetKnown("system_probe_config.max_conns_per_message")
	config.SetKnown("system_probe_config.max_tracked_connections")
	config.SetKnown("system_probe_config.max_closed_connections_buffered")
	config.SetKnown("system_probe_config.max_connection_state_buffered")
	config.SetKnown("system_probe_config.excluded_linux_versions")

	// APM
	config.SetKnown("apm_config.enabled")
	config.SetKnown("apm_config.env")
	config.SetKnown("apm_config.additional_endpoints.*")
	config.SetKnown("apm_config.apm_non_local_traffic")
	config.SetKnown("apm_config.max_traces_per_second")
	config.SetKnown("apm_config.max_memory")
	config.SetKnown("apm_config.log_file")
	config.SetKnown("apm_config.apm_dd_url")
	config.SetKnown("apm_config.max_cpu_percent")
	config.SetKnown("apm_config.receiver_port")
	config.SetKnown("apm_config.receiver_socket")
	config.SetKnown("apm_config.connection_limit")
	config.SetKnown("apm_config.ignore_resources")
	config.SetKnown("apm_config.replace_tags")
	config.SetKnown("apm_config.obfuscation.elasticsearch.enabled")
	config.SetKnown("apm_config.obfuscation.elasticsearch.keep_values")
	config.SetKnown("apm_config.obfuscation.mongodb.enabled")
	config.SetKnown("apm_config.obfuscation.mongodb.keep_values")
	config.SetKnown("apm_config.obfuscation.http.remove_query_string")
	config.SetKnown("apm_config.obfuscation.http.remove_paths_with_digits")
	config.SetKnown("apm_config.obfuscation.remove_stack_traces")
	config.SetKnown("apm_config.obfuscation.redis.enabled")
	config.SetKnown("apm_config.obfuscation.memcached.enabled")
	config.SetKnown("apm_config.extra_sample_rate")
	config.SetKnown("apm_config.dd_agent_bin")
	config.SetKnown("apm_config.max_events_per_second")
	config.SetKnown("apm_config.trace_writer.connection_limit")
	config.SetKnown("apm_config.trace_writer.queue_size")
	config.SetKnown("apm_config.service_writer.connection_limit")
	config.SetKnown("apm_config.service_writer.queue_size")
	config.SetKnown("apm_config.stats_writer.connection_limit")
	config.SetKnown("apm_config.stats_writer.queue_size")
	config.SetKnown("apm_config.analyzed_rate_by_service.*")
	config.SetKnown("apm_config.analyzed_spans.*")
	config.SetKnown("apm_config.log_throttling")
	config.SetKnown("apm_config.bucket_size_seconds")
	config.SetKnown("apm_config.receiver_timeout")
	config.SetKnown("apm_config.watchdog_check_delay")

	setAssetFs(config)
}

var (
	ddURLs = map[string]interface{}{
		"app.datadoghq.com": nil,
		"app.datadoghq.eu":  nil,
		"app.datad0g.com":   nil,
		"app.datad0g.eu":    nil,
	}
)

// GetProxies returns the proxy settings from the configuration
func GetProxies() *Proxy {
	return proxies
}

// loadProxyFromEnv overrides the proxy settings with environment variables
func loadProxyFromEnv(config Config) {
	// Viper doesn't handle mixing nested variables from files and set
	// manually.  If we manually set one of the sub value for "proxy" all
	// other values from the conf file will be shadowed when using
	// 'config.Get("proxy")'. For that reason we first get the value from
	// the conf files, overwrite them with the env variables and reset
	// everything.

	lookupEnvCaseInsensitive := func(key string) (string, bool) {
		value, found := os.LookupEnv(key)
		if !found {
			value, found = os.LookupEnv(strings.ToLower(key))
		}
		if found {
			log.Infof("Found '%v' env var, using it for the Agent proxy settings", key)
		}
		return value, found
	}

	lookupEnv := func(key string) (string, bool) {
		value, found := os.LookupEnv(key)
		if found {
			log.Infof("Found '%v' env var, using it for the Agent proxy settings", key)
		}
		return value, found
	}

	var isSet bool
	p := &Proxy{}
	if isSet = config.IsSet("proxy"); isSet {
		if err := config.UnmarshalKey("proxy", p); err != nil {
			isSet = false
			log.Errorf("Could not load proxy setting from the configuration (ignoring): %s", err)
		}
	}

	if HTTP, found := lookupEnv("DD_PROXY_HTTP"); found {
		isSet = true
		p.HTTP = HTTP
	} else if HTTP, found := lookupEnvCaseInsensitive("HTTP_PROXY"); found {
		isSet = true
		p.HTTP = HTTP
	}

	if HTTPS, found := lookupEnv("DD_PROXY_HTTPS"); found {
		isSet = true
		p.HTTPS = HTTPS
	} else if HTTPS, found := lookupEnvCaseInsensitive("HTTPS_PROXY"); found {
		isSet = true
		p.HTTPS = HTTPS
	}

	if noProxy, found := lookupEnv("DD_PROXY_NO_PROXY"); found {
		isSet = true
		p.NoProxy = strings.Split(noProxy, " ") // space-separated list, consistent with viper
	} else if noProxy, found := lookupEnvCaseInsensitive("NO_PROXY"); found {
		isSet = true
		p.NoProxy = strings.Split(noProxy, ",") // comma-separated list, consistent with other tools that use the NO_PROXY env var
	}

	// We have to set each value individually so both config.Get("proxy")
	// and config.Get("proxy.http") work
	if isSet {
		config.Set("proxy.http", p.HTTP)
		config.Set("proxy.https", p.HTTPS)
		config.Set("proxy.no_proxy", p.NoProxy)
		proxies = p
	}
}

// Load reads configs files and initializes the config module
func Load() error {
	return load(Datadog, "datadog.yaml", true)
}

// LoadWithoutSecret reads configs files, initializes the config module without decrypting any secrets
func LoadWithoutSecret() error {
	return load(Datadog, "datadog.yaml", false)
}

func findUnknownKeys(config Config) []string {
	var unknownKeys []string
	knownKeys := config.GetKnownKeys()
	loadedKeys := config.AllKeys()
	for _, key := range loadedKeys {
		if _, found := knownKeys[key]; !found {
			// Check if any subkey terminated with a '.*' wildcard is marked as known
			// e.g.: apm_config.* would match all sub-keys of apm_config
			splitPath := strings.Split(key, ".")
			for j := range splitPath {
				subKey := strings.Join(splitPath[:j+1], ".") + ".*"
				if _, found = knownKeys[subKey]; found {
					break
				}
			}
			if !found {
				unknownKeys = append(unknownKeys, key)
			}
		}
	}
	return unknownKeys
}

func load(config Config, origin string, loadSecret bool) error {
	if err := config.ReadInConfig(); err != nil {
		log.Warnf("Error loading config: %v", err)
		return err
	}

	for _, key := range findUnknownKeys(config) {
		log.Warnf("Unknown key in config file: %v", key)
	}

	if loadSecret {
		if err := ResolveSecrets(config, origin); err != nil {
			return err
		}
	}

	loadProxyFromEnv(config)
	sanitizeAPIKey(config)
	applyOverrides(config)
	return nil
}

// ResolveSecrets merges all the secret values from origin into config. Secret values
// are identified by a value of the form "ENC[key]" where key is the secret key.
// See: https://github.com/DataDog/datadog-agent/blob/master/docs/agent/secrets.md
func ResolveSecrets(config Config, origin string) error {
	// We have to init the secrets package before we can use it to decrypt
	// anything.
	secrets.Init(
		config.GetString("secret_backend_command"),
		config.GetStringSlice("secret_backend_arguments"),
		config.GetInt("secret_backend_timeout"),
		config.GetInt("secret_backend_output_max_size"),
	)

	if config.GetString("secret_backend_command") != "" {
		// Viper doesn't expose the final location of the file it
		// loads. Since we are searching for 'datadog.yaml' in multiple
		// locations we let viper determine the one to use before
		// updating it.
		yamlConf, err := yaml.Marshal(config.AllSettings())
		if err != nil {
			return fmt.Errorf("unable to marshal configuration to YAML to decrypt secrets: %v", err)
		}

		finalYamlConf, err := secrets.Decrypt(yamlConf, origin)
		if err != nil {
			return fmt.Errorf("unable to decrypt secret from datadog.yaml: %v", err)
		}
		r := bytes.NewReader(finalYamlConf)
		if err = config.MergeConfigOverride(r); err != nil {
			return fmt.Errorf("could not update main configuration after decrypting secrets: %v", err)
		}
	}
	return nil
}

// Avoid log ingestion breaking because of a newline in the API key
func sanitizeAPIKey(config Config) {
	config.Set("api_key", strings.TrimSpace(config.GetString("api_key")))
}

// GetMainInfraEndpoint returns the main DD Infra URL defined in the config, based on the value of `site` and `dd_url`
func GetMainInfraEndpoint() string {
	return getMainInfraEndpointWithConfig(Datadog)
}

// GetMainEndpoint returns the main DD URL defined in the config, based on `site` and the prefix, or ddURLKey
func GetMainEndpoint(prefix string, ddURLKey string) string {
	return GetMainEndpointWithConfig(Datadog, prefix, ddURLKey)
}

// GetMultipleEndpoints returns the api keys per domain specified in the main agent config
func GetMultipleEndpoints() (map[string][]string, error) {
	return getMultipleEndpointsWithConfig(Datadog)
}

// getDomainPrefix provides the right prefix for agent X.Y.Z
func getDomainPrefix(app string) string {
	v, _ := version.New(version.AgentVersion, version.Commit)
	return fmt.Sprintf("%d-%d-%d-%s.agent", v.Major, v.Minor, v.Patch, app)
}

// AddAgentVersionToDomain prefixes the domain with the agent version: X-Y-Z.domain
func AddAgentVersionToDomain(DDURL string, app string) (string, error) {
	u, err := url.Parse(DDURL)
	if err != nil {
		return "", err
	}

	// we don't udpdate unknown URL (ie: proxy or custom StatsD server)
	if _, found := ddURLs[u.Host]; !found {
		return DDURL, nil
	}

	subdomain := strings.Split(u.Host, ".")[0]
	newSubdomain := getDomainPrefix(app)

	u.Host = strings.Replace(u.Host, subdomain, newSubdomain, 1)
	return u.String(), nil
}

func getMainInfraEndpointWithConfig(config Config) string {
	return GetMainEndpointWithConfig(config, infraURLPrefix, "dd_url")
}

// GetMainEndpointWithConfig implements the logic to extract the DD URL from a config, based on `site` and ddURLKey
func GetMainEndpointWithConfig(config Config, prefix string, ddURLKey string) (resolvedDDURL string) {
	if config.IsSet(ddURLKey) && config.GetString(ddURLKey) != "" {
		// value under ddURLKey takes precedence over 'site'
		resolvedDDURL = config.GetString(ddURLKey)
		if config.IsSet("site") {
			log.Infof("'site' and '%s' are both set in config: setting main endpoint to '%s': \"%s\"", ddURLKey, ddURLKey, config.GetString(ddURLKey))
		}
	} else if config.GetString("site") != "" {
		resolvedDDURL = prefix + strings.TrimSpace(config.GetString("site"))
	} else {
		resolvedDDURL = prefix + DefaultSite
	}
	return
}

// getMultipleEndpointsWithConfig implements the logic to extract the api keys per domain from an agent config
func getMultipleEndpointsWithConfig(config Config) (map[string][]string, error) {
	// Validating domain
	ddURL := getMainInfraEndpointWithConfig(config)
	_, err := url.Parse(ddURL)
	if err != nil {
		return nil, fmt.Errorf("could not parse main endpoint: %s", err)
	}

	keysPerDomain := map[string][]string{
		ddURL: {
			config.GetString("api_key"),
		},
	}

	var additionalEndpoints map[string][]string
	err = config.UnmarshalKey("additional_endpoints", &additionalEndpoints)
	if err != nil {
		return keysPerDomain, err
	}

	// merge additional endpoints into keysPerDomain
	for domain, apiKeys := range additionalEndpoints {

		// Validating domain
		_, err := url.Parse(domain)
		if err != nil {
			return nil, fmt.Errorf("could not parse url from 'additional_endpoints' %s: %s", domain, err)
		}

		if _, ok := keysPerDomain[domain]; ok {
			for _, apiKey := range apiKeys {
				keysPerDomain[domain] = append(keysPerDomain[domain], apiKey)
			}
		} else {
			keysPerDomain[domain] = apiKeys
		}
	}

	// dedupe api keys and remove domains with no api keys (or empty ones)
	for domain, apiKeys := range keysPerDomain {
		dedupedAPIKeys := make([]string, 0, len(apiKeys))
		seen := make(map[string]bool)
		for _, apiKey := range apiKeys {
			trimmedAPIKey := strings.TrimSpace(apiKey)
			if _, ok := seen[trimmedAPIKey]; !ok && trimmedAPIKey != "" {
				seen[trimmedAPIKey] = true
				dedupedAPIKeys = append(dedupedAPIKeys, trimmedAPIKey)
			}
		}

		if len(dedupedAPIKeys) > 0 {
			keysPerDomain[domain] = dedupedAPIKeys
		} else {
			log.Infof("No API key provided for domain \"%s\", removing domain from endpoints", domain)
			delete(keysPerDomain, domain)
		}
	}

	return keysPerDomain, nil
}

// IsContainerized returns whether the Agent is running on a Docker container
func IsContainerized() bool {
	return os.Getenv("DOCKER_DD_AGENT") != ""
}

// FileUsedDir returns the absolute path to the folder containing the config
// file used to populate the registry
func FileUsedDir() string {
	return filepath.Dir(Datadog.ConfigFileUsed())
}

// IsKubernetes returns whether the Agent is running on a kubernetes cluster
func IsKubernetes() bool {
	// Injected by Kubernetes itself
	if os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		return true
	}
	// support of Datadog environment variable for Kubernetes
	if os.Getenv("KUBERNETES") != "" {
		return true
	}
	return false
}

// SetOverrides provides an externally accessible method for
// overriding config variables.
// This method must be called before Load() to be effective.
func SetOverrides(vars map[string]interface{}) {
	overrideVars = vars
}

// applyOverrides overrides config variables.
func applyOverrides(config Config) {
	for k, v := range overrideVars {
		config.Set(k, v)
	}
}
