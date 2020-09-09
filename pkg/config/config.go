// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// DefaultSite is the default site the Agent sends data to.
	DefaultSite    = "datadoghq.com"
	infraURLPrefix = "https://app."

	// DefaultNumWorkers default number of workers for our check runner
	DefaultNumWorkers = 4
	// MaxNumWorkers maximum number of workers for our check runner
	MaxNumWorkers = 25
	// DefaultAPIKeyValidationInterval is the default interval of api key validation checks
	DefaultAPIKeyValidationInterval = 60

	// DefaultForwarderRecoveryInterval is the default recovery interval,
	// also used if the user-provided value is invalid.
	DefaultForwarderRecoveryInterval = 2

	megaByte = 1024 * 1024

	// DefaultBatchWait is the default HTTP batch wait in second for logs
	DefaultBatchWait = 5

	// ClusterIDCacheKey is the key name for the orchestrator cluster id in the agent in-mem cache
	ClusterIDCacheKey = "orchestratorClusterID"

	// DefaultRuntimePoliciesDir is the default policies directory used by the runtime security module
	DefaultRuntimePoliciesDir = "/etc/datadog-agent/runtime-security.d"
)

var overrideVars = make(map[string]interface{})

// Datadog is the global configuration object
var (
	Datadog Config
	proxies *Proxy
)

// Variables to initialize at build time
var (
	DefaultPython string

	// ForceDefaultPython has its value set to true at compile time if we should ignore
	// the Python version set in the configuration and use `DefaultPython` instead.
	// We use this to force Python 3 in the Agent 7 as it's the only one available.
	ForceDefaultPython string
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

// MappingProfile represent a group of mappings
type MappingProfile struct {
	Name     string          `mapstructure:"name"`
	Prefix   string          `mapstructure:"prefix"`
	Mappings []MetricMapping `mapstructure:"mappings"`
}

// MetricMapping represent one mapping rule
type MetricMapping struct {
	Match     string            `mapstructure:"match"`
	MatchType string            `mapstructure:"match_type"`
	Name      string            `mapstructure:"name"`
	Tags      map[string]string `mapstructure:"tags"`
}

// Warnings represent the warnings in the config
type Warnings struct {
	TraceMallocEnabledWithPy2 bool
}

func init() {
	osinit()
	// Configure Datadog global configuration
	Datadog = NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	// Configuration defaults
	InitConfig(Datadog)
}

// InitConfig initializes the config defaults on a config
func InitConfig(config Config) {
	// Agent
	// Don't set a default on 'site' to allow detecting with viper whether it's set in config
	config.BindEnv("site")   //nolint:errcheck
	config.BindEnv("dd_url") //nolint:errcheck
	config.BindEnvAndSetDefault("app_key", "")
	config.BindEnvAndSetDefault("cloud_provider_metadata", []string{"aws", "gcp", "azure", "alibaba"})
	config.SetDefault("proxy", nil)
	config.BindEnvAndSetDefault("skip_ssl_validation", false)
	config.BindEnvAndSetDefault("hostname", "")
	config.BindEnvAndSetDefault("tags", []string{})
	config.BindEnv("env") //nolint:errcheck
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
	config.BindEnvAndSetDefault("ipc_address", "localhost")
	config.BindEnvAndSetDefault("health_port", int64(0))
	config.BindEnvAndSetDefault("disable_py3_validation", false)
	config.BindEnvAndSetDefault("python_version", DefaultPython)

	// overridden in IoT Agent main
	config.BindEnvAndSetDefault("iot_host", false)
	// overridden in Heroku buildpack
	config.BindEnvAndSetDefault("heroku_dyno", false)

	// Debugging + C-land crash feature flags
	config.BindEnvAndSetDefault("c_stacktrace_collection", false)
	config.BindEnvAndSetDefault("c_core_dump", false)
	config.BindEnvAndSetDefault("memtrack_enabled", true)
	config.BindEnvAndSetDefault("tracemalloc_debug", false)
	config.BindEnvAndSetDefault("tracemalloc_whitelist", "")
	config.BindEnvAndSetDefault("tracemalloc_blacklist", "")
	config.BindEnvAndSetDefault("run_path", defaultRunPath)

	// Python 3 linter timeout, in seconds
	// NOTE: linter is notoriously slow, in the absence of a better solution we
	//       can only increase this timeout value. Linting operation is async.
	config.BindEnvAndSetDefault("python3_linter_timeout", 120)

	// Whether to honour the value of PYTHONPATH, if set, on Windows. On other OSes we always do.
	config.BindEnvAndSetDefault("windows_use_pythonpath", false)

	// if/when the default is changed to true, make the default platform
	// dependent; default should remain false on Windows to maintain backward
	// compatibility with Agent5 behavior/win
	config.BindEnvAndSetDefault("hostname_fqdn", false)

	// When enabled, hostname defined in the configuration (datadog.yaml) and starting with `ip-` or `domu` on EC2 is used as
	// canonical hostname, otherwise the instance-id is used as canonical hostname.
	config.BindEnvAndSetDefault("hostname_force_config_as_canonical", false)

	config.BindEnvAndSetDefault("cluster_name", "")
	config.BindEnvAndSetDefault("disable_cluster_name_tag_key", false)

	// secrets backend
	config.BindEnvAndSetDefault("secret_backend_command", "")
	config.BindEnvAndSetDefault("secret_backend_arguments", []string{})
	config.BindEnvAndSetDefault("secret_backend_output_max_size", secrets.SecretBackendOutputMaxSize)
	config.BindEnvAndSetDefault("secret_backend_timeout", 5)

	// Use to output logs in JSON format
	config.BindEnvAndSetDefault("log_format_json", false)

	// IPC API server timeout
	config.BindEnvAndSetDefault("server_timeout", 15)

	// Use to force client side TLS version to 1.2
	config.BindEnvAndSetDefault("force_tls_12", false)

	// Defaults to safe YAML methods in base and custom checks.
	config.BindEnvAndSetDefault("disable_unsafe_yaml", true)

	// Yaml keys which values are stripped from flare
	config.BindEnvAndSetDefault("flare_stripped_keys", []string{})

	// Agent GUI access port
	config.BindEnvAndSetDefault("GUI_port", defaultGuiPort)

	if IsContainerized() {
		// In serverless-containerized environments (e.g Fargate)
		// it's impossible to mount host volumes.
		// Make sure the host paths exist before setting-up the default values.
		// Fallback to the container paths if host paths aren't mounted.
		if pathExists("/host/proc") {
			config.SetDefault("procfs_path", "/host/proc")
			config.SetDefault("container_proc_root", "/host/proc")

			// Used by some librairies (like gopsutil)
			if v := os.Getenv("HOST_PROC"); v == "" {
				os.Setenv("HOST_PROC", "/host/proc")
			}
		} else {
			config.SetDefault("procfs_path", "/proc")
			config.SetDefault("container_proc_root", "/proc")
		}
		if pathExists("/host/sys/fs/cgroup/") {
			config.SetDefault("container_cgroup_root", "/host/sys/fs/cgroup/")
		} else {
			config.SetDefault("container_cgroup_root", "/sys/fs/cgroup/")
		}
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

	config.BindEnv("procfs_path")           //nolint:errcheck
	config.BindEnv("container_proc_root")   //nolint:errcheck
	config.BindEnv("container_cgroup_root") //nolint:errcheck

	config.BindEnvAndSetDefault("proc_root", "/proc")
	config.BindEnvAndSetDefault("histogram_aggregates", []string{"max", "median", "avg", "count"})
	config.BindEnvAndSetDefault("histogram_percentiles", []string{"0.95"})
	config.BindEnvAndSetDefault("aggregator_stop_timeout", 2)
	config.BindEnvAndSetDefault("aggregator_buffer_size", 100)
	// Serializer
	config.BindEnvAndSetDefault("enable_stream_payload_serialization", true)
	config.BindEnvAndSetDefault("enable_service_checks_stream_payload_serialization", true)
	config.BindEnvAndSetDefault("enable_events_stream_payload_serialization", true)

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
	config.BindEnvAndSetDefault("additional_endpoints", map[string][]string{})
	config.BindEnvAndSetDefault("forwarder_timeout", 20)
	config.BindEnvAndSetDefault("forwarder_retry_queue_max_size", 30)
	config.BindEnvAndSetDefault("forwarder_connection_reset_interval", 0)                                // in seconds, 0 means disabled
	config.BindEnvAndSetDefault("forwarder_apikey_validation_interval", DefaultAPIKeyValidationInterval) // in minutes
	config.BindEnvAndSetDefault("forwarder_num_workers", 1)
	config.BindEnvAndSetDefault("forwarder_stop_timeout", 2)
	// Forwarder retry settings
	config.BindEnvAndSetDefault("forwarder_backoff_factor", 2)
	config.BindEnvAndSetDefault("forwarder_backoff_base", 2)
	config.BindEnvAndSetDefault("forwarder_backoff_max", 64)
	config.BindEnvAndSetDefault("forwarder_recovery_interval", DefaultForwarderRecoveryInterval)
	config.BindEnvAndSetDefault("forwarder_recovery_reset", false)

	// Dogstatsd
	config.BindEnvAndSetDefault("use_dogstatsd", true)
	config.BindEnvAndSetDefault("dogstatsd_port", 8125)            // Notice: 0 means UDP port closed
	config.BindEnvAndSetDefault("dogstatsd_windows_pipe_name", "") // experimental and not officially supported for now.

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
	config.BindEnvAndSetDefault("dogstatsd_socket", "") // Notice: empty means feature disabled
	config.BindEnvAndSetDefault("dogstatsd_stats_port", 5000)
	config.BindEnvAndSetDefault("dogstatsd_stats_enable", false)
	config.BindEnvAndSetDefault("dogstatsd_stats_buffer", 10)
	config.BindEnvAndSetDefault("dogstatsd_expiry_seconds", 300)
	config.BindEnvAndSetDefault("dogstatsd_origin_detection", false) // Only supported for socket traffic
	config.BindEnvAndSetDefault("dogstatsd_so_rcvbuf", 0)
	config.BindEnvAndSetDefault("dogstatsd_metrics_stats_enable", false)
	config.BindEnvAndSetDefault("dogstatsd_tags", []string{})
	config.BindEnvAndSetDefault("dogstatsd_mapper_cache_size", 1000)
	config.BindEnvAndSetDefault("dogstatsd_string_interner_size", 4096)
	// Enable check for Entity-ID presence when enriching Dogstatsd metrics with tags
	config.BindEnvAndSetDefault("dogstatsd_entity_id_precedence", false)
	// Sends Dogstatsd parse errors to the Debug level instead of the Error level
	config.BindEnvAndSetDefault("dogstatsd_disable_verbose_logs", false)
	config.SetKnown("dogstatsd_mapper_profiles")

	config.BindEnvAndSetDefault("statsd_forward_host", "")
	config.BindEnvAndSetDefault("statsd_forward_port", 0)
	config.BindEnvAndSetDefault("statsd_metric_namespace", "")
	config.BindEnvAndSetDefault("statsd_metric_namespace_blacklist", StandardStatsdPrefixes)
	// Autoconfig
	config.BindEnvAndSetDefault("autoconf_template_dir", "/datadog/check_configs")
	config.BindEnvAndSetDefault("exclude_pause_container", true)
	config.BindEnvAndSetDefault("ac_include", []string{})
	config.BindEnvAndSetDefault("ac_exclude", []string{})
	config.BindEnvAndSetDefault("container_include", []string{})
	config.BindEnvAndSetDefault("container_exclude", []string{})
	config.BindEnvAndSetDefault("container_include_metrics", []string{})
	config.BindEnvAndSetDefault("container_exclude_metrics", []string{})
	config.BindEnvAndSetDefault("container_include_logs", []string{})
	config.BindEnvAndSetDefault("container_exclude_logs", []string{})
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
	config.BindEnvAndSetDefault("kubernetes_kubelet_nodename", "")
	config.BindEnvAndSetDefault("eks_fargate", false)
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

	// SNMP
	config.SetKnown("snmp_listener.discovery_interval")
	config.SetKnown("snmp_listener.allowed_failures")
	config.SetKnown("snmp_listener.workers")
	config.SetKnown("snmp_listener.configs")

	config.BindEnvAndSetDefault("snmp_traps_enabled", false)
	config.BindEnvAndSetDefault("snmp_traps_config.port", 162)
	config.BindEnvAndSetDefault("snmp_traps_config.community_strings", []string{})
	config.BindEnvAndSetDefault("snmp_traps_config.bind_host", "localhost")
	config.BindEnvAndSetDefault("snmp_traps_config.stop_timeout", 5) // in seconds

	// Kube ApiServer
	config.BindEnvAndSetDefault("kubernetes_kubeconfig_path", "")
	config.BindEnvAndSetDefault("leader_lease_duration", "60")
	config.BindEnvAndSetDefault("leader_election", false)
	config.BindEnvAndSetDefault("kube_resources_namespace", "")
	config.BindEnvAndSetDefault("cache_sync_timeout", 2) // in seconds

	// Datadog cluster agent
	config.BindEnvAndSetDefault("cluster_agent.enabled", false)
	config.BindEnvAndSetDefault("cluster_agent.auth_token", "")
	config.BindEnvAndSetDefault("cluster_agent.url", "")
	config.BindEnvAndSetDefault("cluster_agent.kubernetes_service_name", "datadog-cluster-agent")
	config.BindEnvAndSetDefault("cluster_agent.tagging_fallback", false)
	config.BindEnvAndSetDefault("metrics_port", "5000")

	// Metadata endpoints

	// Defines the maximum size of hostame gathered from EC2, GCE, Azure and Alibabacloud metadata endpoints.
	// Used internally to protect against configurations where metadata endpoints return incorrect values with 200 status codes.
	config.BindEnvAndSetDefault("metadata_endpoints_max_hostname_size", 255)

	// EC2
	config.BindEnvAndSetDefault("ec2_use_windows_prefix_detection", false)
	config.BindEnvAndSetDefault("ec2_metadata_timeout", 300)          // value in milliseconds
	config.BindEnvAndSetDefault("ec2_metadata_token_lifetime", 21600) // value in seconds
	config.BindEnvAndSetDefault("ec2_prefer_imdsv2", false)
	config.BindEnvAndSetDefault("collect_ec2_tags", false)

	// ECS
	config.BindEnvAndSetDefault("ecs_agent_url", "") // Will be autodetected
	config.BindEnvAndSetDefault("ecs_agent_container_name", "ecs-agent")
	config.BindEnvAndSetDefault("ecs_collect_resource_tags_ec2", false)

	// GCE
	config.BindEnvAndSetDefault("collect_gce_tags", true)
	config.BindEnvAndSetDefault("exclude_gce_tags", []string{"kube-env", "kubelet-config", "containerd-configure-sh", "startup-script", "shutdown-script", "configure-sh", "sshKeys", "ssh-keys", "user-data", "cli-cert", "ipsec-cert", "ssl-cert", "google-container-manifest", "bosh_settings", "windows-startup-script-ps1", "common-psm1", "k8s-node-setup-psm1", "serial-port-logging-enable", "enable-oslogin", "disable-address-manager", "disable-legacy-endpoints", "windows-keys"})
	config.BindEnvAndSetDefault("gce_metadata_timeout", 1000) // value in milliseconds

	// Cloud Foundry
	config.BindEnvAndSetDefault("cloud_foundry", false)
	config.BindEnvAndSetDefault("bosh_id", "")
	config.BindEnvAndSetDefault("cf_os_hostname_aliasing", false)

	// Cloud Foundry BBS
	config.BindEnvAndSetDefault("cloud_foundry_bbs.url", "https://bbs.service.cf.internal:8889")
	config.BindEnvAndSetDefault("cloud_foundry_bbs.poll_interval", 15)
	config.BindEnvAndSetDefault("cloud_foundry_bbs.ca_file", "")
	config.BindEnvAndSetDefault("cloud_foundry_bbs.cert_file", "")
	config.BindEnvAndSetDefault("cloud_foundry_bbs.key_file", "")

	// Cloud Foundry Garden
	config.BindEnvAndSetDefault("cloud_foundry_garden.listen_network", "unix")
	config.BindEnvAndSetDefault("cloud_foundry_garden.listen_address", "/var/vcap/data/garden/garden.sock")

	// JMXFetch
	config.BindEnvAndSetDefault("jmx_custom_jars", []string{})
	config.BindEnvAndSetDefault("jmx_use_cgroup_memory_limit", false)
	config.BindEnvAndSetDefault("jmx_use_container_support", false)
	config.BindEnvAndSetDefault("jmx_max_restarts", int64(3))
	config.BindEnvAndSetDefault("jmx_restart_interval", int64(5))
	config.BindEnvAndSetDefault("jmx_thread_pool_size", 3)
	config.BindEnvAndSetDefault("jmx_reconnection_thread_pool_size", 3)
	config.BindEnvAndSetDefault("jmx_collection_timeout", 60)
	config.BindEnvAndSetDefault("jmx_check_period", int(defaults.DefaultCheckInterval/time.Millisecond))
	config.BindEnvAndSetDefault("jmx_reconnection_timeout", 60)

	// Go_expvar server port
	config.BindEnvAndSetDefault("expvar_port", "5000")

	// Profiling
	config.BindEnvAndSetDefault("profiling.enabled", false)
	config.BindEnv("profiling.profile_dd_url", "") //nolint:errcheck

	// Trace agent
	// Note that trace-agent environment variables are parsed in pkg/trace/config/env.go
	// since some of them require custom parsing algorithms. DO NOT add environment variable
	// bindings here, add them there instead.
	if runtime.GOARCH == "386" && runtime.GOOS == "windows" {
		// on Windows-32 bit, the trace agent isn't installed.  Set the default to disabled
		// so that there aren't messages in the log about failing to start.
		config.BindEnvAndSetDefault("apm_config.enabled", false)
	} else {
		config.BindEnvAndSetDefault("apm_config.enabled", true)
	}

	// Process agent
	config.SetDefault("process_config.enabled", "false")
	// process_config.enabled is only used on Windows by the core agent to start the process agent service.
	// it can be set from file, but not from env. Override it with value from DD_PROCESS_AGENT_ENABLED.
	ddProcessAgentEnabled, found := os.LookupEnv("DD_PROCESS_AGENT_ENABLED")
	if found {
		overrideVars["process_config.enabled"] = ddProcessAgentEnabled
	}

	config.BindEnv("process_config.process_dd_url", "") //nolint:errcheck

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
	config.BindEnv("logs_config.logs_dd_url") //nolint:errcheck // must respect format '<HOST>:<PORT>' and '<PORT>' to be an integer
	// specific logs-agent api-key
	config.BindEnv("logs_config.api_key") //nolint:errcheck
	config.BindEnvAndSetDefault("logs_config.logs_no_ssl", false)
	// send the logs to the port 443 of the logs-backend via TCP:
	config.BindEnvAndSetDefault("logs_config.use_port_443", false)
	// increase the read buffer size of the UDP sockets:
	config.BindEnvAndSetDefault("logs_config.frame_size", 9000)
	// increase the number of files that can be tailed in parallel:
	config.BindEnvAndSetDefault("logs_config.open_files_limit", 100)
	// add global processing rules that are applied on all logs
	config.BindEnv("logs_config.processing_rules") //nolint:errcheck
	// enforce the agent to use files to collect container logs on kubernetes environment
	config.BindEnvAndSetDefault("logs_config.k8s_container_use_file", false)
	// additional config to ensure initial logs are tagged with kubelet tags
	// wait (seconds) for tagger before start fetching tags of new AD services
	config.BindEnvAndSetDefault("logs_config.tagger_warmup_duration", 0) // Disabled by default (0 seconds)
	// Configurable docker client timeout while communicating with the docker daemon.
	// It could happen that the docker daemon takes a lot of time gathering timestamps
	// before starting to send any data when it has stored several large log files.
	// This field lets you increase the read timeout to prevent the client from
	// timing out too early in such a situation. Value in seconds.
	config.BindEnvAndSetDefault("logs_config.docker_client_read_timeout", 30)
	// Internal Use Only: avoid modifying those configuration parameters, this could lead to unexpected results.
	config.BindEnvAndSetDefault("logs_config.run_path", defaultRunPath)
	config.BindEnv("logs_config.dd_url") //nolint:errcheck
	config.BindEnvAndSetDefault("logs_config.use_http", false)
	config.BindEnvAndSetDefault("logs_config.use_tcp", false)
	config.BindEnvAndSetDefault("logs_config.use_compression", true)
	config.BindEnvAndSetDefault("logs_config.compression_level", 6) // Default level for the gzip/deflate algorithm
	config.BindEnvAndSetDefault("logs_config.batch_wait", DefaultBatchWait)
	config.BindEnvAndSetDefault("logs_config.connection_reset_interval", 0) // in seconds, 0 means disabled
	config.BindEnvAndSetDefault("logs_config.dd_port", 10516)
	config.BindEnvAndSetDefault("logs_config.dev_mode_use_proto", true)
	config.BindEnvAndSetDefault("logs_config.dd_url_443", "agent-443-intake.logs.datadoghq.com")
	config.BindEnvAndSetDefault("logs_config.stop_grace_period", 30)
	config.BindEnv("logs_config.additional_endpoints") //nolint:errcheck

	// The cardinality of tags to send for checks and dogstatsd respectively.
	// Choices are: low, orchestrator, high.
	// WARNING: sending orchestrator, or high tags for dogstatsd metrics may create more metrics
	// (one per container instead of one per host).
	// Changing this setting may impact your custom metrics billing.
	config.BindEnvAndSetDefault("checks_tag_cardinality", "low")
	config.BindEnvAndSetDefault("dogstatsd_tag_cardinality", "low")

	config.BindEnvAndSetDefault("histogram_copy_to_distribution", false)
	config.BindEnvAndSetDefault("histogram_copy_to_distribution_prefix", "")

	config.BindEnv("api_key") //nolint:errcheck

	config.BindEnvAndSetDefault("hpa_watcher_polling_freq", 10)
	config.BindEnvAndSetDefault("hpa_watcher_gc_period", 60*5) // 5 minutes
	config.BindEnvAndSetDefault("external_metrics_provider.enabled", false)
	config.BindEnvAndSetDefault("external_metrics_provider.port", 443)
	config.BindEnvAndSetDefault("hpa_configmap_name", "datadog-custom-metrics")
	config.BindEnvAndSetDefault("external_metrics_provider.refresh_period", 30)           // value in seconds. Frequency of calls to Datadog to refresh metric values
	config.BindEnvAndSetDefault("external_metrics_provider.batch_window", 10)             // value in seconds. Batch the events from the Autoscalers informer to push updates to the ConfigMap (GlobalStore)
	config.BindEnvAndSetDefault("external_metrics_provider.max_age", 120)                 // value in seconds. 4 cycles from the Autoscaler controller (up to Kubernetes 1.11) is enough to consider a metric stale
	config.BindEnvAndSetDefault("external_metrics.aggregator", "avg")                     // aggregator used for the external metrics. Choose from [avg,sum,max,min]
	config.BindEnvAndSetDefault("external_metrics_provider.bucket_size", 60*5)            // Window to query to get the metric from Datadog.
	config.BindEnvAndSetDefault("external_metrics_provider.rollup", 30)                   // Bucket size to circumvent time aggregation side effects.
	config.BindEnvAndSetDefault("external_metrics_provider.wpa_controller", false)        // Activates the controller for Watermark Pod Autoscalers.
	config.BindEnvAndSetDefault("external_metrics_provider.use_datadogmetric_crd", false) // Use DatadogMetric CRD with custom Datadog Queries instead of ConfigMap
	config.BindEnvAndSetDefault("kubernetes_event_collection_timeout", 100)               // timeout between two successful event collections in milliseconds.
	config.BindEnvAndSetDefault("kubernetes_informers_resync_period", 60*5)               // value in seconds. Default to 5 minutes
	config.BindEnvAndSetDefault("external_metrics_provider.local_copy_refresh_rate", 30)  // value in seconds
	// Cluster check Autodiscovery
	config.BindEnvAndSetDefault("cluster_checks.enabled", false)
	config.BindEnvAndSetDefault("cluster_checks.node_expiration_timeout", 30) // value in seconds
	config.BindEnvAndSetDefault("cluster_checks.warmup_duration", 30)         // value in seconds
	config.BindEnvAndSetDefault("cluster_checks.cluster_tag_name", "cluster_name")
	config.BindEnvAndSetDefault("cluster_checks.extra_tags", []string{})
	config.BindEnvAndSetDefault("cluster_checks.advanced_dispatching_enabled", false)
	config.BindEnvAndSetDefault("cluster_checks.clc_runners_port", 5005)
	// Cluster check runner
	config.BindEnvAndSetDefault("clc_runner_enabled", false)
	config.BindEnvAndSetDefault("clc_runner_host", "") // must be set using the Kubernetes downward API
	config.BindEnvAndSetDefault("clc_runner_port", 5005)
	config.BindEnvAndSetDefault("clc_runner_server_write_timeout", 15)
	config.BindEnvAndSetDefault("clc_runner_server_readheader_timeout", 10)
	// Admission controller
	config.BindEnvAndSetDefault("admission_controller.enabled", false)
	config.BindEnvAndSetDefault("admission_controller.mutate_unlabelled", false)
	config.BindEnvAndSetDefault("admission_controller.port", 8000)
	config.BindEnvAndSetDefault("admission_controller.service_name", "datadog-admission-controller")
	config.BindEnvAndSetDefault("admission_controller.certificate.validity_bound", 365*24)             // validity bound of the certificate created by the controller (in hours, default 1 year)
	config.BindEnvAndSetDefault("admission_controller.certificate.expiration_threshold", 30*24)        // how long before its expiration a certificate should be refreshed (in hours, default 1 month)
	config.BindEnvAndSetDefault("admission_controller.certificate.secret_name", "webhook-certificate") // name of the Secret object containing the webhook certificate
	config.BindEnvAndSetDefault("admission_controller.webhook_name", "datadog-webhook")
	config.BindEnvAndSetDefault("admission_controller.inject_config.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.inject_config.endpoint", "/injectconfig")
	config.BindEnvAndSetDefault("admission_controller.inject_tags.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.inject_tags.endpoint", "/injecttags")
	config.BindEnvAndSetDefault("admission_controller.pod_owners_cache_validity", 10) // in minutes

	// Telemetry
	// Enable telemetry metrics on the internals of the Agent.
	// This create a lot of billable custom metrics.
	config.BindEnvAndSetDefault("telemetry.enabled", false)
	config.SetKnown("telemetry.checks")

	// Declare other keys that don't have a default/env var.
	// Mostly, keys we use IsSet() on, because IsSet always returns true if a key has a default.
	config.SetKnown("metadata_providers")
	config.SetKnown("config_providers")
	config.SetKnown("cluster_name")
	config.SetKnown("listeners")
	config.SetKnown("proxy.http")
	config.SetKnown("proxy.https")
	config.SetKnown("proxy.no_proxy")

	// Orchestrator Explorer DCA and process-agent
	config.BindEnvAndSetDefault("orchestrator_explorer.enabled", false)
	// enabling/disabling the environment variables & command scrubbing from the container specs
	// this option will potentially impact the CPU usage of the agent
	config.BindEnvAndSetDefault("orchestrator_explorer.container_scrubbing.enabled", true)

	// Orchestrator Explorer - process agent
	config.BindEnv("orchestrator_explorer.orchestrator_dd_url", "") //nolint:errcheck
	// DEPRECATED in favor of `orchestrator_explorer.orchestrator_dd_url` setting. If both are set `orchestrator_explorer.orchestrator_dd_url` will take precedence.
	config.BindEnv("process_config.orchestrator_dd_url", "") //nolint:errcheck
	// DEPRECATED in favor of `orchestrator_explorer.orchestrator_additional_endpoints` setting. If both are set `orchestrator_explorer.orchestrator_additional_endpoints` will take precedence.
	config.SetKnown("process_config.orchestrator_additional_endpoints.*")
	config.SetKnown("orchestrator_explorer.orchestrator_additional_endpoints.*")

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
	config.SetKnown("system_probe_config.bpf_dir")
	config.SetKnown("system_probe_config.disable_tcp")
	config.SetKnown("system_probe_config.disable_udp")
	config.SetKnown("system_probe_config.disable_ipv6")
	config.SetKnown("system_probe_config.disable_dns_inspection")
	config.SetKnown("system_probe_config.collect_local_dns")
	config.SetKnown("system_probe_config.use_local_system_probe")
	config.SetKnown("system_probe_config.enable_conntrack")
	config.SetKnown("system_probe_config.sysprobe_socket")
	config.SetKnown("system_probe_config.conntrack_rate_limit")
	config.SetKnown("system_probe_config.max_conns_per_message")
	config.SetKnown("system_probe_config.max_tracked_connections")
	config.SetKnown("system_probe_config.max_closed_connections_buffered")
	config.SetKnown("system_probe_config.max_connection_state_buffered")
	config.SetKnown("system_probe_config.excluded_linux_versions")
	config.SetKnown("system_probe_config.source_excludes")
	config.SetKnown("system_probe_config.dest_excludes")
	config.SetKnown("system_probe_config.closed_channel_size")
	config.SetKnown("system_probe_config.dns_timeout_in_s")
	config.SetKnown("system_probe_config.collect_dns_stats")
	config.SetKnown("system_probe_config.offset_guess_threshold")
	config.SetKnown("system_probe_config.enable_tcp_queue_length")
	config.SetKnown("system_probe_config.enable_oom_kill")
	config.SetKnown("system_probe_config.enable_tracepoints")
	config.SetKnown("system_probe_config.windows.enable_monotonic_count")
	config.SetKnown("system_probe_config.windows.driver_buffer_size")

	// Network
	config.BindEnv("network.id") //nolint:errcheck

	// APM
	config.SetKnown("apm_config.enabled")
	config.SetKnown("apm_config.env")
	config.SetKnown("apm_config.additional_endpoints.*")
	config.SetKnown("apm_config.apm_non_local_traffic")
	config.SetKnown("apm_config.max_traces_per_second")
	config.SetKnown("apm_config.max_memory")
	config.SetKnown("apm_config.log_file")
	config.SetKnown("apm_config.apm_dd_url")
	config.SetKnown("apm_config.profiling_dd_url")
	config.SetKnown("apm_config.profiling_additional_endpoints.*")
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
	config.SetKnown("apm_config.connection_reset_interval") // in seconds
	config.SetKnown("apm_config.analyzed_rate_by_service.*")
	config.SetKnown("apm_config.analyzed_spans.*")
	config.SetKnown("apm_config.log_throttling")
	config.SetKnown("apm_config.bucket_size_seconds")
	config.SetKnown("apm_config.receiver_timeout")
	config.SetKnown("apm_config.watchdog_check_delay")
	config.SetKnown("apm_config.max_payload_size")

	// inventories
	config.BindEnvAndSetDefault("inventories_enabled", true)
	config.BindEnvAndSetDefault("inventories_max_interval", 600) // 10min
	config.BindEnvAndSetDefault("inventories_min_interval", 300) // 5min

	// Datadog security agent (common)
	config.BindEnvAndSetDefault("security_agent.cmd_port", 5010)
	config.BindEnvAndSetDefault("security_agent.expvar_port", 5011)
	config.BindEnvAndSetDefault("security_agent.log_file", defaultSecurityAgentLogFile)

	// Datadog security agent (compliance)
	config.BindEnvAndSetDefault("compliance_config.enabled", false)
	config.BindEnvAndSetDefault("compliance_config.check_interval", 20*time.Minute)
	config.BindEnvAndSetDefault("compliance_config.dir", "/etc/datadog-agent/compliance.d")
	config.BindEnvAndSetDefault("compliance_config.run_path", defaultRunPath)

	// Datadog security agent (runtime)
	config.BindEnvAndSetDefault("runtime_security_config.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.debug", false)
	config.BindEnvAndSetDefault("runtime_security_config.policies.dir", DefaultRuntimePoliciesDir)
	config.BindEnvAndSetDefault("runtime_security_config.socket", "/opt/datadog-agent/run/runtime-security.sock")
	config.BindEnvAndSetDefault("runtime_security_config.enable_kernel_filters", true)
	config.BindEnvAndSetDefault("runtime_security_config.syscall_monitor.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.run_path", defaultRunPath)

	// command line options
	config.SetKnown("cmd.check.fullsketches")

	setAssetFs(config)
}

var (
	ddURLRegexp = regexp.MustCompile(`^app(\.(us|eu)\d)?\.datad(oghq|0g)\.(com|eu)$`)
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
func Load() (*Warnings, error) {
	return load(Datadog, "datadog.yaml", true)
}

// LoadWithoutSecret reads configs files, initializes the config module without decrypting any secrets
func LoadWithoutSecret() (*Warnings, error) {
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

func load(config Config, origin string, loadSecret bool) (*Warnings, error) {
	warnings := Warnings{}

	if err := config.ReadInConfig(); err != nil {
		if errors.Is(err, os.ErrPermission) {
			log.Warnf("Error loading config: %v (check config file permissions for dd-agent user)", err)
		} else {
			log.Warnf("Error loading config: %v", err)
		}
		return &warnings, err
	}

	for _, key := range findUnknownKeys(config) {
		log.Warnf("Unknown key in config file: %v", key)
	}

	if loadSecret {
		if err := ResolveSecrets(config, origin); err != nil {
			return &warnings, err
		}
	}

	// If this variable is set to true, we'll use DefaultPython for the Python version,
	// ignoring the python_version configuration value.
	if ForceDefaultPython == "true" {
		override := make(map[string]interface{})
		override["python_version"] = DefaultPython

		pv := config.GetString("python_version")
		if pv != DefaultPython {
			log.Warnf("Python version has been forced to %s", DefaultPython)
		}

		AddOverrides(override)
	}

	loadProxyFromEnv(config)
	SanitizeAPIKeyConfig(config, "api_key")
	applyOverrides(config)
	// setTracemallocEnabled *must* be called before setNumWorkers
	warnings.TraceMallocEnabledWithPy2 = setTracemallocEnabled(config)
	setNumWorkers(config)
	return &warnings, nil
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

// SanitizeAPIKeyConfig strips newlines and other control characters from a given key.
func SanitizeAPIKeyConfig(config Config, key string) {
	config.Set(key, SanitizeAPIKey(config.GetString(key)))
}

// SanitizeAPIKey strips newlines and other control characters from a given string.
func SanitizeAPIKey(key string) string {
	return strings.TrimSpace(key)
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
	v, _ := version.Agent()
	return fmt.Sprintf("%d-%d-%d-%s.agent", v.Major, v.Minor, v.Patch, app)
}

// AddAgentVersionToDomain prefixes the domain with the agent version: X-Y-Z.domain
func AddAgentVersionToDomain(DDURL string, app string) (string, error) {
	u, err := url.Parse(DDURL)
	if err != nil {
		return "", err
	}

	// we don't update unknown URLs (ie: proxy or custom DD domain)
	if !ddURLRegexp.MatchString(u.Host) {
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
		resolvedDDURL = getResolvedDDUrl(config, ddURLKey)
	} else if config.GetString("site") != "" {
		resolvedDDURL = prefix + strings.TrimSpace(config.GetString("site"))
	} else {
		resolvedDDURL = prefix + DefaultSite
	}
	return
}

// GetMainEndpointWithConfigBackwardCompatible implements the logic to extract the DD URL from a config, based on `site`,ddURLKey and a backward compatible key
func GetMainEndpointWithConfigBackwardCompatible(config Config, prefix string, ddURLKey string, backwardKey string) (resolvedDDURL string) {
	if config.IsSet(ddURLKey) && config.GetString(ddURLKey) != "" {
		// value under ddURLKey takes precedence over backwardKey and 'site'
		resolvedDDURL = getResolvedDDUrl(config, ddURLKey)
	} else if config.IsSet(backwardKey) && config.GetString(backwardKey) != "" {
		// value under backwardKey takes precedence over 'site'
		resolvedDDURL = getResolvedDDUrl(config, backwardKey)
	} else if config.GetString("site") != "" {
		resolvedDDURL = prefix + strings.TrimSpace(config.GetString("site"))
	} else {
		resolvedDDURL = prefix + DefaultSite
	}
	return
}

func getResolvedDDUrl(config Config, urlKey string) string {
	resolvedDDURL := config.GetString(urlKey)
	if config.IsSet("site") {
		log.Infof("'site' and '%s' are both set in config: setting main endpoint to '%s': \"%s\"", urlKey, urlKey, config.GetString(urlKey))
	}
	return resolvedDDURL
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

	additionalEndpoints := config.GetStringMapStringSlice("additional_endpoints")
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

// IsCloudProviderEnabled checks the cloud provider family provided in
// pkg/util/<cloud_provider>.go against the value for cloud_provider: on the
// global config object Datadog
func IsCloudProviderEnabled(cloudProviderName string) bool {
	cloudProviderFromConfig := Datadog.GetStringSlice("cloud_provider_metadata")

	for _, cloudName := range cloudProviderFromConfig {
		if strings.ToLower(cloudName) == strings.ToLower(cloudProviderName) {
			log.Debugf("cloud_provider_metadata is set to %s in agent configuration, trying endpoints for %s Cloud Provider",
				cloudProviderFromConfig,
				cloudProviderName)
			return true
		}
	}

	log.Debugf("cloud_provider_metadata is set to %s in agent configuration, skipping %s Cloud Provider",
		cloudProviderFromConfig,
		cloudProviderName)
	return false
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

// GetIPCAddress returns the IPC address or an error if the address is not local
func GetIPCAddress() (string, error) {
	address := Datadog.GetString("ipc_address")
	if address == "localhost" {
		return address, nil
	}
	ip := net.ParseIP(address)
	if ip == nil {
		return "", fmt.Errorf("ipc_address was set to an invalid IP address: %s", address)
	}
	for _, cidr := range []string{
		"127.0.0.0/8", // IPv4 loopback
		"::1/128",     // IPv6 loopback
	} {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", err
		}
		if block.Contains(ip) {
			return address, nil
		}
	}
	return "", fmt.Errorf("ipc_address was set to a non-loopback IP address: %s", address)
}

// GetEnv retrieves the value of the environment variable named by the key,
// or def if the environment variable was not set.
func GetEnv(key, def string) string {
	value, found := os.LookupEnv(key)
	if !found {
		return def
	}
	return value
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

// pathExists returns true if the given path exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// AddOverrides provides an externally accessible method for
// overriding config variables.
// This method must be called before Load() to be effective.
func AddOverrides(vars map[string]interface{}) {
	for k, v := range vars {
		overrideVars[k] = v
	}
}

// applyOverrides overrides config variables.
func applyOverrides(config Config) {
	for k, v := range overrideVars {
		config.Set(k, v)
	}
}

// setTracemallocEnabled is a helper to get the effective tracemalloc
// configuration.
func setTracemallocEnabled(config Config) bool {
	pyVersion := config.GetString("python_version")
	wTracemalloc := config.GetBool("tracemalloc_debug")
	traceMallocEnabledWithPy2 := false
	if pyVersion == "2" && wTracemalloc {
		log.Warnf("Tracemalloc was enabled but unavailable with python version %q, disabling.", pyVersion)
		wTracemalloc = false
		traceMallocEnabledWithPy2 = true
	}

	// update config with the actual effective tracemalloc
	config.Set("tracemalloc_debug", wTracemalloc)
	return traceMallocEnabledWithPy2
}

// setNumWorkers is a helper to set the effective number of workers for
// a given config.
func setNumWorkers(config Config) {
	wTracemalloc := config.GetBool("tracemalloc_debug")
	numWorkers := config.GetInt("check_runners")
	if wTracemalloc {
		log.Infof("Tracemalloc enabled, only one check runner enabled to run checks serially")
		numWorkers = 1
	}

	if numWorkers > MaxNumWorkers {
		numWorkers = MaxNumWorkers
		log.Warnf("Configured number of checks workers (%v) is too high: %v will be used", numWorkers, MaxNumWorkers)
	}

	// update config with the actual effective number of workers
	config.Set("check_runners", numWorkers)
}

// GetDogstatsdMappingProfiles returns mapping profiles used in DogStatsD mapper
func GetDogstatsdMappingProfiles() ([]MappingProfile, error) {
	return getDogstatsdMappingProfilesConfig(Datadog)
}

func getDogstatsdMappingProfilesConfig(config Config) ([]MappingProfile, error) {
	var mappings []MappingProfile
	if config.IsSet("dogstatsd_mapper_profiles") {
		err := config.UnmarshalKey("dogstatsd_mapper_profiles", &mappings)
		if err != nil {
			return []MappingProfile{}, log.Errorf("Could not parse dogstatsd_mapper_profiles: %v", err)
		}
	}
	return mappings, nil
}

// IsCLCRunner returns whether the Agent is in cluster check runner mode
func IsCLCRunner() bool {
	if !Datadog.GetBool("clc_runner_enabled") {
		return false
	}
	var cp []ConfigurationProviders
	if err := Datadog.UnmarshalKey("config_providers", &cp); err == nil {
		for _, name := range Datadog.GetStringSlice("extra_config_providers") {
			cp = append(cp, ConfigurationProviders{Name: name})
		}
		if len(cp) == 1 && cp[0].Name == "clusterchecks" {
			// A cluster check runner is an Agent configured to run clusterchecks only
			return true
		}
	}
	return false
}
