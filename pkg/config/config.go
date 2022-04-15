// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
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

	// DefaultBatchMaxConcurrentSend is the default HTTP batch max concurrent send for logs
	DefaultBatchMaxConcurrentSend = 0

	// DefaultBatchMaxSize is the default HTTP batch max size (maximum number of events in a single batch) for logs
	DefaultBatchMaxSize = 1000

	// DefaultBatchMaxContentSize is the default HTTP batch max content size (before compression) for logs
	// It is also the maximum possible size of a single event. Events exceeding this limit are dropped.
	DefaultBatchMaxContentSize = 5000000

	// DefaultAuditorTTL is the default logs auditor TTL in hours
	DefaultAuditorTTL = 23

	// ClusterIDCacheKey is the key name for the orchestrator cluster id in the agent in-mem cache
	ClusterIDCacheKey = "orchestratorClusterID"

	// DefaultRuntimePoliciesDir is the default policies directory used by the runtime security module
	DefaultRuntimePoliciesDir = "/etc/datadog-agent/runtime-security.d"

	// DefaultLogsSenderBackoffFactor is the default logs sender backoff randomness factor
	DefaultLogsSenderBackoffFactor = 2.0

	// DefaultLogsSenderBackoffBase is the default logs sender base backoff time, seconds
	DefaultLogsSenderBackoffBase = 1.0

	// DefaultLogsSenderBackoffMax is the default logs sender maximum backoff time, seconds
	DefaultLogsSenderBackoffMax = 120.0

	// DefaultLogsSenderBackoffRecoveryInterval is the default logs sender backoff recovery interval
	DefaultLogsSenderBackoffRecoveryInterval = 2

	// DefaultInventoriesMinInterval is the default value for inventories_min_interval, in seconds
	DefaultInventoriesMinInterval = 5 * 60

	// DefaultInventoriesMaxInterval is the default value for inventories_max_interval, in seconds
	DefaultInventoriesMaxInterval = 10 * 60

	// maxExternalMetricsProviderChunkSize ensures batch queries are limited in size.
	maxExternalMetricsProviderChunkSize = 35
)

// Datadog is the global configuration object
var (
	Datadog       Config
	proxies       *Proxy
	overrideFuncs = make([]func(Config), 0)
)

// Variables to initialize at build time
var (
	DefaultPython string

	// ForceDefaultPython has its value set to true at compile time if we should ignore
	// the Python version set in the configuration and use `DefaultPython` instead.
	// We use this to force Python 3 in the Agent 7 as it's the only one available.
	ForceDefaultPython string
)

// Variables to initialize at start time
var (
	// StartTime is the agent startup time
	StartTime = time.Now()

	// PrometheusScrapeChecksTransformer decodes the `prometheus_scrape.checks` parameter
	PrometheusScrapeChecksTransformer func(string) interface{}
)

// MetadataProviders helps unmarshalling `metadata_providers` config param
type MetadataProviders struct {
	Name     string        `mapstructure:"name"`
	Interval time.Duration `mapstructure:"interval"`
}

// ConfigurationProviders helps unmarshalling `config_providers` config param
type ConfigurationProviders struct {
	Name                    string `mapstructure:"name"`
	Polling                 bool   `mapstructure:"polling"`
	PollInterval            string `mapstructure:"poll_interval"`
	TemplateURL             string `mapstructure:"template_url"`
	TemplateDir             string `mapstructure:"template_dir"`
	Username                string `mapstructure:"username"`
	Password                string `mapstructure:"password"`
	CAFile                  string `mapstructure:"ca_file"`
	CAPath                  string `mapstructure:"ca_path"`
	CertFile                string `mapstructure:"cert_file"`
	KeyFile                 string `mapstructure:"key_file"`
	Token                   string `mapstructure:"token"`
	GraceTimeSeconds        int    `mapstructure:"grace_time_seconds"`
	DegradedDeadlineMinutes int    `mapstructure:"degraded_deadline_minutes"`
}

// Listeners helps unmarshalling `listeners` config param
type Listeners struct {
	Name             string `mapstructure:"name"`
	EnabledProviders map[string]struct{}
}

// SetEnabledProviders registers the enabled config providers in the listener config
func (l *Listeners) SetEnabledProviders(ep map[string]struct{}) {
	l.EnabledProviders = ep
}

// IsProviderEnabled returns whether a config provider is enabled
func (l *Listeners) IsProviderEnabled(provider string) bool {
	_, found := l.EnabledProviders[provider]

	return found
}

// Proxy represents the configuration for proxies in the agent
type Proxy struct {
	HTTP    string   `mapstructure:"http"`
	HTTPS   string   `mapstructure:"https"`
	NoProxy []string `mapstructure:"no_proxy"`
}

// MappingProfile represent a group of mappings
type MappingProfile struct {
	Name     string          `mapstructure:"name" json:"name"`
	Prefix   string          `mapstructure:"prefix" json:"prefix"`
	Mappings []MetricMapping `mapstructure:"mappings" json:"mappings"`
}

// MetricMapping represent one mapping rule
type MetricMapping struct {
	Match     string            `mapstructure:"match" json:"match"`
	MatchType string            `mapstructure:"match_type" json:"match_type"`
	Name      string            `mapstructure:"name" json:"name"`
	Tags      map[string]string `mapstructure:"tags" json:"tags"`
}

// Endpoint represent a datadog endpoint
type Endpoint struct {
	Site   string `mapstructure:"site" json:"site"`
	URL    string `mapstructure:"url" json:"url"`
	APIKey string `mapstructure:"api_key" json:"api_key"`
	APPKey string `mapstructure:"app_key" json:"app_key" `
}

// Warnings represent the warnings in the config
type Warnings struct {
	TraceMallocEnabledWithPy2 bool
}

// DataType represent the generic data type (e.g. metrics, logs) that can be sent by the Agent
type DataType string

const (
	// Metrics type covers series & sketches
	Metrics DataType = "metrics"
	// Logs type covers all outgoing logs
	Logs DataType = "logs"
)

// prometheusScrapeChecksTransformer is a trampoline function that delays the
// resolution of `PrometheusScrapeChecksTransformer` from `InitConfig` (invoked
// from an `init()` function to `cobra.(*Command).Execute` (invoked from `main.main`)
//
// Without it, the issue is that `config.PrometheusScrapeChecksTransformer` would be:
// * written from an `init` function from `pkg/autodiscovery/common/types` and
// * read from an `init` function here.
// This would result in an undefined behaviour
//
// With this intermediate function, it’s read from `cobra.(*Command).Execute`
// which is called from `main.main` which is guaranteed to be called after all `init`.
func prometheusScrapeChecksTransformer(s string) interface{} {
	return PrometheusScrapeChecksTransformer(s)
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
	config.BindEnv("site")
	config.BindEnv("dd_url", "DD_DD_URL", "DD_URL")
	config.BindEnvAndSetDefault("app_key", "")
	config.BindEnvAndSetDefault("cloud_provider_metadata", []string{"aws", "gcp", "azure", "alibaba", "oracle", "ibm"})
	config.SetDefault("proxy", nil)
	config.BindEnvAndSetDefault("skip_ssl_validation", false)
	config.BindEnvAndSetDefault("sslkeylogfile", "")
	config.BindEnvAndSetDefault("hostname", "")
	config.BindEnvAndSetDefault("hostname_file", "")
	config.BindEnvAndSetDefault("tags", []string{})
	config.BindEnvAndSetDefault("extra_tags", []string{})
	config.BindEnv("env")
	config.BindEnvAndSetDefault("tag_value_split_separator", map[string]string{})
	config.BindEnvAndSetDefault("conf_path", ".")
	config.BindEnvAndSetDefault("confd_path", defaultConfdPath)
	config.BindEnvAndSetDefault("additional_checksd", defaultAdditionalChecksPath)
	config.BindEnvAndSetDefault("jmx_log_file", "")
	config.BindEnvAndSetDefault("log_payloads", false)
	config.BindEnvAndSetDefault("log_file", "")
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
	config.BindEnvAndSetDefault("syslog_pem", "")
	config.BindEnvAndSetDefault("syslog_key", "")
	config.BindEnvAndSetDefault("syslog_tls_verify", true)
	config.BindEnvAndSetDefault("cmd_host", "localhost")
	config.BindEnvAndSetDefault("cmd_port", 5001)
	config.BindEnvAndSetDefault("default_integration_http_timeout", 9)
	config.BindEnvAndSetDefault("integration_tracing", false)
	config.BindEnvAndSetDefault("enable_metadata_collection", true)
	config.BindEnvAndSetDefault("enable_gohai", true)
	config.BindEnvAndSetDefault("check_runners", int64(4))
	config.BindEnvAndSetDefault("auth_token_file_path", "")
	config.BindEnv("bind_host")
	config.BindEnvAndSetDefault("ipc_address", "localhost")
	config.BindEnvAndSetDefault("health_port", int64(0))
	config.BindEnvAndSetDefault("disable_py3_validation", false)
	config.BindEnvAndSetDefault("python_version", DefaultPython)
	config.BindEnvAndSetDefault("allow_arbitrary_tags", false)
	config.BindEnvAndSetDefault("use_proxy_for_cloud_metadata", false)
	config.BindEnvAndSetDefault("remote_tagger_timeout_seconds", 30)

	// Remote config
	config.BindEnvAndSetDefault("remote_configuration.enabled", false)
	config.BindEnvAndSetDefault("remote_configuration.key", "")
	config.BindEnv("remote_configuration.api_key")
	config.BindEnv("remote_configuration.rc_dd_url", "")
	config.BindEnvAndSetDefault("remote_configuration.config_root", "")
	config.BindEnvAndSetDefault("remote_configuration.director_root", "")
	config.BindEnvAndSetDefault("remote_configuration.refresh_interval", 1*time.Minute)
	config.BindEnvAndSetDefault("remote_configuration.max_backoff_interval", 5*time.Minute)
	config.BindEnvAndSetDefault("remote_configuration.clients.ttl_seconds", 30*time.Second)
	// Remote config products
	config.BindEnvAndSetDefault("remote_configuration.apm_sampling.enabled", true)

	// Auto exit configuration
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
	config.BindEnvAndSetDefault("host_aliases", []string{})

	// overridden in IoT Agent main
	config.BindEnvAndSetDefault("iot_host", false)
	// overridden in Heroku buildpack
	config.BindEnvAndSetDefault("heroku_dyno", false)

	// Debugging + C-land crash feature flags
	config.BindEnvAndSetDefault("c_stacktrace_collection", false)
	config.BindEnvAndSetDefault("c_core_dump", false)
	config.BindEnvAndSetDefault("go_core_dump", false)
	config.BindEnvAndSetDefault("memtrack_enabled", true)
	config.BindEnvAndSetDefault("tracemalloc_debug", false)
	config.BindEnvAndSetDefault("tracemalloc_include", "")
	config.BindEnvAndSetDefault("tracemalloc_exclude", "")
	config.BindEnvAndSetDefault("tracemalloc_whitelist", "") // deprecated
	config.BindEnvAndSetDefault("tracemalloc_blacklist", "") // deprecated
	config.BindEnvAndSetDefault("run_path", defaultRunPath)
	config.BindEnvAndSetDefault("no_proxy_nonexact_match", false)

	// Python 3 linter timeout, in seconds
	// NOTE: linter is notoriously slow, in the absence of a better solution we
	//       can only increase this timeout value. Linting operation is async.
	config.BindEnvAndSetDefault("python3_linter_timeout", 120)

	// Whether to honour the value of PYTHONPATH, if set, on Windows. On other OSes we always do.
	config.BindEnvAndSetDefault("windows_use_pythonpath", false)

	// When the Python full interpreter path cannot be deduced via heuristics, the agent
	// is expected to prevent rtloader from initializing. When set to true, this override
	// allows us to proceed but with some capabilities unavailable (e.g. `multiprocessing`
	// library support will not work reliably in those environments)
	config.BindEnvAndSetDefault("allow_python_path_heuristics_failure", false)

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
	config.BindEnvAndSetDefault("secret_backend_timeout", 30)
	config.BindEnvAndSetDefault("secret_backend_command_allow_group_exec_perm", false)
	config.BindEnvAndSetDefault("secret_backend_skip_checks", false)

	// Use to output logs in JSON format
	config.BindEnvAndSetDefault("log_format_json", false)

	// IPC API server timeout
	config.BindEnvAndSetDefault("server_timeout", 30)

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

	config.BindEnv("procfs_path")
	config.BindEnv("container_proc_root")
	config.BindEnv("container_cgroup_root")
	config.BindEnvAndSetDefault("ignore_host_etc", false)

	config.BindEnvAndSetDefault("proc_root", "/proc")
	config.BindEnvAndSetDefault("histogram_aggregates", []string{"max", "median", "avg", "count"})
	config.BindEnvAndSetDefault("histogram_percentiles", []string{"0.95"})
	config.BindEnvAndSetDefault("aggregator_stop_timeout", 2)
	config.BindEnvAndSetDefault("aggregator_buffer_size", 100)
	config.BindEnvAndSetDefault("aggregator_use_tags_store", true)
	config.BindEnvAndSetDefault("basic_telemetry_add_container_tags", false) // configure adding the agent container tags to the basic agent telemetry metrics (e.g. `datadog.agent.running`)
	config.BindEnvAndSetDefault("aggregator_flush_metrics_and_serialize_in_parallel_chan_size", 200)
	config.BindEnvAndSetDefault("aggregator_flush_metrics_and_serialize_in_parallel_buffer_size", 4000)

	// Serializer
	config.BindEnvAndSetDefault("enable_stream_payload_serialization", true)
	config.BindEnvAndSetDefault("enable_service_checks_stream_payload_serialization", true)
	config.BindEnvAndSetDefault("enable_events_stream_payload_serialization", true)
	config.BindEnvAndSetDefault("enable_sketch_stream_payload_serialization", true)
	config.BindEnvAndSetDefault("enable_json_stream_shared_compressor_buffers", true)

	// Warning: do not change the following values. Your payloads will get dropped by Datadog's intake.
	config.BindEnvAndSetDefault("serializer_max_payload_size", 2*megaByte+megaByte/2)
	config.BindEnvAndSetDefault("serializer_max_uncompressed_payload_size", 4*megaByte)
	config.BindEnvAndSetDefault("serializer_max_series_points_per_payload", 10000)
	config.BindEnvAndSetDefault("serializer_max_series_payload_size", 512000)
	config.BindEnvAndSetDefault("serializer_max_series_uncompressed_payload_size", 5242880)

	config.BindEnvAndSetDefault("use_v2_api.series", false)
	// Serializer: allow user to blacklist any kind of payload to be sent
	config.BindEnvAndSetDefault("enable_payloads.events", true)
	config.BindEnvAndSetDefault("enable_payloads.series", true)
	config.BindEnvAndSetDefault("enable_payloads.service_checks", true)
	config.BindEnvAndSetDefault("enable_payloads.sketches", true)
	config.BindEnvAndSetDefault("enable_payloads.json_to_v1_intake", true)

	// Forwarder
	config.BindEnvAndSetDefault("additional_endpoints", map[string][]string{})
	config.BindEnvAndSetDefault("forwarder_timeout", 20)
	config.BindEnv("forwarder_retry_queue_max_size")                                                     // Deprecated in favor of `forwarder_retry_queue_payloads_max_size`
	config.BindEnv("forwarder_retry_queue_payloads_max_size")                                            // Default value is defined inside `NewOptions` in pkg/forwarder/forwarder.go
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

	// Forwarder storage on disk
	config.BindEnvAndSetDefault("forwarder_storage_path", "")
	config.BindEnvAndSetDefault("forwarder_outdated_file_in_days", 10)
	config.BindEnvAndSetDefault("forwarder_flush_to_disk_mem_ratio", 0.5)
	config.BindEnvAndSetDefault("forwarder_storage_max_size_in_bytes", 0)                // 0 means disabled. This is a BETA feature.
	config.BindEnvAndSetDefault("forwarder_storage_max_disk_ratio", 0.80)                // Do not store transactions on disk when the disk usage exceeds 80% of the disk capacity. Use 80% as some applications do not behave well when the disk space is very small.
	config.BindEnvAndSetDefault("forwarder_retry_queue_capacity_time_interval_sec", 900) // 15 mins

	// Forwarder channels buffer size
	config.BindEnvAndSetDefault("forwarder_high_prio_buffer_size", 100)
	config.BindEnvAndSetDefault("forwarder_low_prio_buffer_size", 100)
	config.BindEnvAndSetDefault("forwarder_requeue_buffer_size", 100)

	// Dogstatsd
	config.BindEnvAndSetDefault("use_dogstatsd", true)
	config.BindEnvAndSetDefault("dogstatsd_port", 8125)    // Notice: 0 means UDP port closed
	config.BindEnvAndSetDefault("dogstatsd_pipe_name", "") // experimental and not officially supported for now.
	// Experimental and not officially supported for now.
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
	config.BindEnvAndSetDefault("dogstatsd_socket", "") // Notice: empty means feature disabled
	config.BindEnvAndSetDefault("dogstatsd_pipeline_autoadjust", false)
	config.BindEnvAndSetDefault("dogstatsd_pipeline_count", 1)
	config.BindEnvAndSetDefault("dogstatsd_stats_port", 5000)
	config.BindEnvAndSetDefault("dogstatsd_stats_enable", false)
	config.BindEnvAndSetDefault("dogstatsd_stats_buffer", 10)
	// Control for how long counter would be sampled to 0 if not received
	config.BindEnvAndSetDefault("dogstatsd_expiry_seconds", 300)
	// Control how long we keep dogstatsd contexts in memory. This should
	// not be set bellow 2 dogstatsd bucket size (ie 20s, since each bucket
	// is 10s), otherwise we won't be able to sample unseen counter as
	// contexts will be deleted (see 'dogstatsd_expiry_seconds').
	config.BindEnvAndSetDefault("dogstatsd_context_expiry_seconds", 300)
	config.BindEnvAndSetDefault("dogstatsd_origin_detection", false) // Only supported for socket traffic
	config.BindEnvAndSetDefault("dogstatsd_origin_detection_client", false)
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

	config.BindEnv("dogstatsd_mapper_profiles")
	config.SetEnvKeyTransformer("dogstatsd_mapper_profiles", func(in string) interface{} {
		var mappings []MappingProfile
		if err := json.Unmarshal([]byte(in), &mappings); err != nil {
			log.Errorf(`"dogstatsd_mapper_profiles" can not be parsed: %v`, err)
		}
		return mappings
	})

	config.BindEnvAndSetDefault("statsd_forward_host", "")
	config.BindEnvAndSetDefault("statsd_forward_port", 0)
	config.BindEnvAndSetDefault("statsd_metric_namespace", "")
	config.BindEnvAndSetDefault("statsd_metric_namespace_blacklist", StandardStatsdPrefixes)
	config.BindEnvAndSetDefault("statsd_metric_blocklist", []string{})
	// Autoconfig
	config.BindEnvAndSetDefault("autoconf_template_dir", "/datadog/check_configs")
	config.BindEnvAndSetDefault("exclude_pause_container", true)
	config.BindEnvAndSetDefault("ac_include", []string{})
	config.BindEnvAndSetDefault("ac_exclude", []string{})
	// ac_load_timeout is used to delay the introduction of sources other than
	// the ones automatically loaded by the AC, into the logs agent.
	// It is mainly here to delay the introduction of the container_collect_all
	// in the logs agent, to avoid it to tail all the available containers.
	config.BindEnvAndSetDefault("ac_load_timeout", 30000) // in milliseconds
	config.BindEnvAndSetDefault("container_include", []string{})
	config.BindEnvAndSetDefault("container_exclude", []string{})
	config.BindEnvAndSetDefault("container_include_metrics", []string{})
	config.BindEnvAndSetDefault("container_exclude_metrics", []string{})
	config.BindEnvAndSetDefault("container_include_logs", []string{})
	config.BindEnvAndSetDefault("container_exclude_logs", []string{})
	config.BindEnvAndSetDefault("container_exclude_stopped_age", DefaultAuditorTTL-1) // in hours
	config.BindEnvAndSetDefault("ad_config_poll_interval", int64(10))                 // in seconds
	config.BindEnvAndSetDefault("extra_listeners", []string{})
	config.BindEnvAndSetDefault("extra_config_providers", []string{})
	config.BindEnvAndSetDefault("ignore_autoconf", []string{})
	config.BindEnvAndSetDefault("autoconfig_from_environment", true)
	config.BindEnvAndSetDefault("autoconfig_exclude_features", []string{})
	config.BindEnvAndSetDefault("autoconfig_include_features", []string{})

	// Docker
	config.BindEnvAndSetDefault("docker_query_timeout", int64(5))
	config.BindEnvAndSetDefault("docker_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("docker_env_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_pod_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_pod_annotations_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_node_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_node_annotations_as_tags", map[string]string{"cluster.k8s.io/machine": "kube_machine"})
	config.BindEnvAndSetDefault("kubernetes_node_annotations_as_host_aliases", []string{"cluster.k8s.io/machine"})
	config.BindEnvAndSetDefault("kubernetes_namespace_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("container_cgroup_prefix", "")

	// CRI
	config.BindEnvAndSetDefault("cri_socket_path", "")              // empty is disabled
	config.BindEnvAndSetDefault("cri_connection_timeout", int64(1)) // in seconds
	config.BindEnvAndSetDefault("cri_query_timeout", int64(5))      // in seconds

	// Containerd
	config.BindEnvAndSetDefault("containerd_namespace", []string{})
	config.BindEnvAndSetDefault("containerd_namespaces", []string{}) // alias for containerd_namespace
	config.BindEnvAndSetDefault("container_env_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("container_labels_as_tags", map[string]string{})

	// Kubernetes
	config.BindEnvAndSetDefault("kubernetes_kubelet_host", "")
	config.BindEnvAndSetDefault("kubernetes_kubelet_nodename", "")
	config.BindEnvAndSetDefault("eks_fargate", false)
	config.BindEnvAndSetDefault("kubernetes_http_kubelet_port", 10255)
	config.BindEnvAndSetDefault("kubernetes_https_kubelet_port", 10250)

	config.BindEnvAndSetDefault("kubelet_tls_verify", true)
	config.BindEnvAndSetDefault("collect_kubernetes_events", false)
	config.BindEnvAndSetDefault("kubelet_client_ca", "")

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

	config.BindEnvAndSetDefault("prometheus_scrape.enabled", false)           // Enables the prometheus config provider
	config.BindEnvAndSetDefault("prometheus_scrape.service_endpoints", false) // Enables Service Endpoints checks in the prometheus config provider
	config.BindEnv("prometheus_scrape.checks")                                // Defines any extra prometheus/openmetrics check configurations to be handled by the prometheus config provider
	config.SetEnvKeyTransformer("prometheus_scrape.checks", prometheusScrapeChecksTransformer)
	config.BindEnvAndSetDefault("prometheus_scrape.version", 1) // Version of the openmetrics check to be scheduled by the Prometheus auto-discovery

	// SNMP
	config.SetKnown("snmp_listener.discovery_interval")
	config.SetKnown("snmp_listener.allowed_failures")
	config.SetKnown("snmp_listener.discovery_allowed_failures")
	config.SetKnown("snmp_listener.collect_device_metadata")
	config.SetKnown("snmp_listener.workers")
	config.SetKnown("snmp_listener.configs")
	config.SetKnown("snmp_listener.loader")
	config.SetKnown("snmp_listener.min_collection_interval")
	config.SetKnown("snmp_listener.namespace")

	config.BindEnvAndSetDefault("snmp_traps_enabled", false)
	config.BindEnvAndSetDefault("snmp_traps_config.port", 9162)
	config.BindEnvAndSetDefault("snmp_traps_config.community_strings", []string{})
	// No default as the agent falls back to `network_devices.namespace` if empty.
	config.BindEnv("snmp_traps_config.namespace")
	config.BindEnvAndSetDefault("snmp_traps_config.bind_host", "0.0.0.0")
	config.BindEnvAndSetDefault("snmp_traps_config.stop_timeout", 5) // in seconds
	config.SetKnown("snmp_traps_config.users")

	// Kube ApiServer
	config.BindEnvAndSetDefault("kubernetes_kubeconfig_path", "")
	config.BindEnvAndSetDefault("kubernetes_apiserver_ca_path", "")
	config.BindEnvAndSetDefault("kubernetes_apiserver_tls_verify", true)
	config.BindEnvAndSetDefault("leader_lease_duration", "60")
	config.BindEnvAndSetDefault("leader_election", false)
	config.BindEnvAndSetDefault("kube_resources_namespace", "")
	config.BindEnvAndSetDefault("kube_cache_sync_timeout_seconds", 5)

	// Datadog cluster agent
	config.BindEnvAndSetDefault("cluster_agent.enabled", false)
	config.BindEnvAndSetDefault("cluster_agent.cmd_port", 5005)
	config.BindEnvAndSetDefault("cluster_agent.allow_legacy_tls", false)
	config.BindEnvAndSetDefault("cluster_agent.auth_token", "")
	config.BindEnvAndSetDefault("cluster_agent.url", "")
	config.BindEnvAndSetDefault("cluster_agent.kubernetes_service_name", "datadog-cluster-agent")
	config.BindEnvAndSetDefault("cluster_agent.tagging_fallback", false)
	config.BindEnvAndSetDefault("cluster_agent.server.read_timeout_seconds", 2)
	config.BindEnvAndSetDefault("cluster_agent.server.write_timeout_seconds", 2)
	config.BindEnvAndSetDefault("cluster_agent.server.idle_timeout_seconds", 60)
	config.BindEnvAndSetDefault("cluster_agent.serve_nozzle_data", false)
	config.BindEnvAndSetDefault("cluster_agent.advanced_tagging", false)
	config.BindEnvAndSetDefault("cluster_agent.max_leader_connections", 500)
	config.BindEnvAndSetDefault("cluster_agent.max_leader_idle_connections", 50)
	config.BindEnvAndSetDefault("cluster_agent.client_reconnect_period_seconds", 1200)
	config.BindEnvAndSetDefault("metrics_port", "5000")

	// Metadata endpoints

	// Defines the maximum size of hostame gathered from EC2, GCE, Azure, Alibaba, Oracle and Tencent cloud metadata
	// endpoints (all cloudprovider except IBM). IBM cloud ignore this setting as their API return a huge JSON with
	// all the metadata for the VM.
	// Used internally to protect against configurations where metadata endpoints return incorrect values with 200 status codes.
	config.BindEnvAndSetDefault("metadata_endpoints_max_hostname_size", 255)

	// EC2
	config.BindEnvAndSetDefault("ec2_use_windows_prefix_detection", false)
	config.BindEnvAndSetDefault("ec2_metadata_timeout", 300)          // value in milliseconds
	config.BindEnvAndSetDefault("ec2_metadata_token_lifetime", 21600) // value in seconds
	config.BindEnvAndSetDefault("ec2_prefer_imdsv2", false)
	config.BindEnvAndSetDefault("collect_ec2_tags", false)
	config.BindEnvAndSetDefault("collect_ec2_tags_use_imds", false)

	// ECS
	config.BindEnvAndSetDefault("ecs_agent_url", "") // Will be autodetected
	config.BindEnvAndSetDefault("ecs_agent_container_name", "ecs-agent")
	config.BindEnvAndSetDefault("ecs_collect_resource_tags_ec2", false)
	config.BindEnvAndSetDefault("ecs_resource_tags_replace_colon", false)
	config.BindEnvAndSetDefault("ecs_metadata_timeout", 500) // value in milliseconds

	// GCE
	config.BindEnvAndSetDefault("collect_gce_tags", true)
	config.BindEnvAndSetDefault("exclude_gce_tags", []string{
		"kube-env", "kubelet-config", "containerd-configure-sh", "startup-script", "shutdown-script",
		"configure-sh", "sshKeys", "ssh-keys", "user-data", "cli-cert", "ipsec-cert", "ssl-cert", "google-container-manifest",
		"bosh_settings", "windows-startup-script-ps1", "common-psm1", "k8s-node-setup-psm1", "serial-port-logging-enable",
		"enable-oslogin", "disable-address-manager", "disable-legacy-endpoints", "windows-keys", "kubeconfig",
	})
	config.BindEnvAndSetDefault("gce_send_project_id_tag", false)
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
	config.BindEnvAndSetDefault("cloud_foundry_bbs.env_include", []string{})
	config.BindEnvAndSetDefault("cloud_foundry_bbs.env_exclude", []string{})

	// Cloud Foundry CC
	config.BindEnvAndSetDefault("cloud_foundry_cc.url", "https://cloud-controller-ng.service.cf.internal:9024")
	config.BindEnvAndSetDefault("cloud_foundry_cc.client_id", "")
	config.BindEnvAndSetDefault("cloud_foundry_cc.client_secret", "")
	config.BindEnvAndSetDefault("cloud_foundry_cc.poll_interval", 60)
	config.BindEnvAndSetDefault("cloud_foundry_cc.skip_ssl_validation", false)
	config.BindEnvAndSetDefault("cloud_foundry_cc.apps_batch_size", 5000)

	// Cloud Foundry Garden
	config.BindEnvAndSetDefault("cloud_foundry_garden.listen_network", "unix")
	config.BindEnvAndSetDefault("cloud_foundry_garden.listen_address", "/var/vcap/data/garden/garden.sock")

	// Azure
	config.BindEnvAndSetDefault("azure_hostname_style", "os")

	// IBM cloud
	// We use a long timeout here since the metadata and token API can be very slow sometimes.
	config.BindEnvAndSetDefault("ibm_metadata_timeout", 5) // value in seconds

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
	config.BindEnvAndSetDefault("jmx_statsd_telemetry_enabled", false)
	// this is an internal setting and will not be documented in the config template.
	// the queue size is the no. of elements (metrics, event, service checks) it can hold.
	config.BindEnvAndSetDefault("jmx_statsd_client_queue_size", 4096)

	// Go_expvar server port
	config.BindEnvAndSetDefault("expvar_port", "5000")

	// internal profiling
	config.BindEnvAndSetDefault("internal_profiling.enabled", false)
	config.BindEnv("internal_profiling.profile_dd_url", "")
	config.BindEnvAndSetDefault("internal_profiling.period", 5*time.Minute)
	config.BindEnvAndSetDefault("internal_profiling.cpu_duration", 1*time.Minute)
	config.BindEnvAndSetDefault("internal_profiling.block_profile_rate", 0)
	config.BindEnvAndSetDefault("internal_profiling.mutex_profile_fraction", 0)
	config.BindEnvAndSetDefault("internal_profiling.enable_goroutine_stacktraces", false)
	// Logs Agent

	// External Use: modify those parameters to configure the logs-agent.
	// enable the logs-agent:
	config.BindEnvAndSetDefault("logs_enabled", false)
	config.BindEnvAndSetDefault("log_enabled", false) // deprecated, use logs_enabled instead
	// collect all logs from all containers:
	config.BindEnvAndSetDefault("logs_config.container_collect_all", false)
	// add a socks5 proxy:
	config.BindEnvAndSetDefault("logs_config.socks5_proxy_address", "")
	// specific logs-agent api-key
	config.BindEnv("logs_config.api_key")

	// Duration during which the host tags will be submitted with log events.
	config.BindEnvAndSetDefault("logs_config.expected_tags_duration", time.Duration(0)) // duration-formatted string (parsed by `time.ParseDuration`)
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
	config.BindEnvAndSetDefault("logs_config.tagger_warmup_duration", 0) // Disabled by default (0 seconds)
	// Configurable docker client timeout while communicating with the docker daemon.
	// It could happen that the docker daemon takes a lot of time gathering timestamps
	// before starting to send any data when it has stored several large log files.
	// This field lets you increase the read timeout to prevent the client from
	// timing out too early in such a situation. Value in seconds.
	config.BindEnvAndSetDefault("logs_config.docker_client_read_timeout", 30)
	// Internal Use Only: avoid modifying those configuration parameters, this could lead to unexpected results.
	config.BindEnvAndSetDefault("logs_config.run_path", defaultRunPath)
	// DEPRECATED in favor of `logs_config.force_use_http`.
	config.BindEnvAndSetDefault("logs_config.use_http", false)
	config.BindEnvAndSetDefault("logs_config.force_use_http", false)
	// DEPRECATED in favor of `logs_config.force_use_tcp`.
	config.BindEnvAndSetDefault("logs_config.use_tcp", false)
	config.BindEnvAndSetDefault("logs_config.force_use_tcp", false)

	bindEnvAndSetLogsConfigKeys(config, "logs_config.")
	bindEnvAndSetLogsConfigKeys(config, "database_monitoring.samples.")
	bindEnvAndSetLogsConfigKeys(config, "database_monitoring.activity.")
	bindEnvAndSetLogsConfigKeys(config, "database_monitoring.metrics.")
	bindEnvAndSetLogsConfigKeys(config, "network_devices.metadata.")
	config.BindEnvAndSetDefault("network_devices.namespace", "default")

	config.BindEnvAndSetDefault("logs_config.dd_port", 10516)
	config.BindEnvAndSetDefault("logs_config.dev_mode_use_proto", true)
	config.BindEnvAndSetDefault("logs_config.dd_url_443", "agent-443-intake.logs.datadoghq.com")
	config.BindEnvAndSetDefault("logs_config.stop_grace_period", 30)
	config.BindEnvAndSetDefault("logs_config.close_timeout", 60)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_detection", false)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_extra_patterns", []string{})
	// The following auto_multi_line settings are experimental and may change
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_default_sample_size", 500)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_default_match_timeout", 30) // Seconds
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_default_match_threshold", 0.48)

	// If true, the agent looks for container logs in the location used by podman, rather
	// than docker.  This is a temporary configuration parameter to support podman logs until
	// a more substantial refactor of autodiscovery is made to determine this automatically.
	config.BindEnvAndSetDefault("logs_config.use_podman_logs", false)

	config.BindEnvAndSetDefault("logs_config.auditor_ttl", DefaultAuditorTTL) // in hours
	// Timeout in milliseonds used when performing agreggation operations,
	// including multi-line log processing rules and chunked line reaggregation.
	// It may be useful to increase it when logs writing is slowed down, that
	// could happen while serializing large objects on log lines.
	config.BindEnvAndSetDefault("logs_config.aggregation_timeout", 1000)
	// Time in seconds
	config.BindEnvAndSetDefault("logs_config.file_scan_period", 10.0)

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
	config.BindEnvAndSetDefault("hpa_configmap_name", "datadog-custom-metrics")
	config.BindEnvAndSetDefault("external_metrics_provider.enabled", false)
	config.BindEnvAndSetDefault("external_metrics_provider.port", 8443)
	config.BindEnvAndSetDefault("external_metrics_provider.endpoint", "")                 // Override the Datadog API endpoint to query external metrics from
	config.BindEnvAndSetDefault("external_metrics_provider.api_key", "")                  // Override the Datadog API Key for external metrics endpoint
	config.BindEnvAndSetDefault("external_metrics_provider.app_key", "")                  // Override the Datadog APP Key for external metrics endpoint
	config.SetKnown("external_metrics_provider.endpoints")                                // List of redundant endpoints to query external metrics from
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
	config.BindEnvAndSetDefault("external_metrics_provider.config", map[string]string{})  // list of options that can be used to configure the external metrics server
	config.BindEnvAndSetDefault("external_metrics_provider.local_copy_refresh_rate", 30)  // value in seconds
	config.BindEnvAndSetDefault("external_metrics_provider.chunk_size", 35)               // Maximum number of queries to batch when querying Datadog.
	AddOverrideFunc(sanitizeExternalMetricsProviderChunkSize)
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
	config.BindEnvAndSetDefault("clc_runner_id", "")
	config.BindEnvAndSetDefault("clc_runner_host", "") // must be set using the Kubernetes downward API
	config.BindEnvAndSetDefault("clc_runner_port", 5005)
	config.BindEnvAndSetDefault("clc_runner_server_write_timeout", 15)
	config.BindEnvAndSetDefault("clc_runner_server_readheader_timeout", 10)
	// Admission controller
	config.BindEnvAndSetDefault("admission_controller.enabled", false)
	config.BindEnvAndSetDefault("admission_controller.mutate_unlabelled", false)
	config.BindEnvAndSetDefault("admission_controller.port", 8000)
	config.BindEnvAndSetDefault("admission_controller.timeout_seconds", 10) // in seconds (see kubernetes/kubernetes#71508)
	config.BindEnvAndSetDefault("admission_controller.service_name", "datadog-admission-controller")
	config.BindEnvAndSetDefault("admission_controller.certificate.validity_bound", 365*24)             // validity bound of the certificate created by the controller (in hours, default 1 year)
	config.BindEnvAndSetDefault("admission_controller.certificate.expiration_threshold", 30*24)        // how long before its expiration a certificate should be refreshed (in hours, default 1 month)
	config.BindEnvAndSetDefault("admission_controller.certificate.secret_name", "webhook-certificate") // name of the Secret object containing the webhook certificate
	config.BindEnvAndSetDefault("admission_controller.webhook_name", "datadog-webhook")
	config.BindEnvAndSetDefault("admission_controller.inject_config.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.inject_config.endpoint", "/injectconfig")
	config.BindEnvAndSetDefault("admission_controller.inject_config.mode", "hostip") // possible values: hostip / service / socket
	config.BindEnvAndSetDefault("admission_controller.inject_config.local_service_name", "datadog")
	config.BindEnvAndSetDefault("admission_controller.inject_config.socket_path", "/var/run/datadog")
	config.BindEnvAndSetDefault("admission_controller.inject_tags.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.inject_tags.endpoint", "/injecttags")
	config.BindEnvAndSetDefault("admission_controller.pod_owners_cache_validity", 10) // in minutes
	config.BindEnvAndSetDefault("admission_controller.namespace_selector_fallback", false)
	config.BindEnvAndSetDefault("admission_controller.failure_policy", "Ignore")
	config.BindEnvAndSetDefault("admission_controller.reinvocation_policy", "IfNeeded")
	config.BindEnvAndSetDefault("admission_controller.add_aks_selectors", false) // adds in the webhook some selectors that are required in AKS

	// Telemetry
	// Enable telemetry metrics on the internals of the Agent.
	// This create a lot of billable custom metrics.
	config.BindEnvAndSetDefault("telemetry.enabled", false)
	config.BindEnv("telemetry.checks")
	// We're using []string as a default instead of []float64 because viper can only parse list of string from the environment
	//
	// The histogram buckets use to track the time in nanoseconds DogStatsD listeners are not reading/waiting new data
	config.BindEnvAndSetDefault("telemetry.dogstatsd.listeners_latency_buckets", []string{})
	// The histogram buckets use to track the time in nanoseconds it takes for the DogStatsD server to push data to the aggregator
	config.BindEnvAndSetDefault("telemetry.dogstatsd.aggregator_channel_latency_buckets", []string{})
	// The histogram buckets use to track the time in nanoseconds it takes for a DogStatsD listeners to push data to the server
	config.BindEnvAndSetDefault("telemetry.dogstatsd.listeners_channel_latency_buckets", []string{})

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
	config.BindEnvAndSetDefault("orchestrator_explorer.custom_sensitive_words", []string{})
	config.BindEnv("orchestrator_explorer.max_per_message")
	config.BindEnv("orchestrator_explorer.orchestrator_dd_url")
	config.BindEnv("orchestrator_explorer.orchestrator_additional_endpoints")
	config.BindEnv("orchestrator_explorer.use_legacy_endpoint")

	// Container lifecycle configuration
	config.BindEnvAndSetDefault("container_lifecycle.enabled", false)
	config.BindEnv("container_lifecycle.dd_url")
	config.BindEnv("container_lifecycle.additional_endpoints")

	// Orchestrator Explorer - process agent
	// DEPRECATED in favor of `orchestrator_explorer.orchestrator_dd_url` setting. If both are set `orchestrator_explorer.orchestrator_dd_url` will take precedence.
	config.BindEnv("process_config.orchestrator_dd_url", "DD_PROCESS_CONFIG_ORCHESTRATOR_DD_URL", "DD_PROCESS_AGENT_ORCHESTRATOR_DD_URL")
	// DEPRECATED in favor of `orchestrator_explorer.orchestrator_additional_endpoints` setting. If both are set `orchestrator_explorer.orchestrator_additional_endpoints` will take precedence.
	config.SetKnown("process_config.orchestrator_additional_endpoints.*")
	config.SetKnown("orchestrator_explorer.orchestrator_additional_endpoints.*")
	config.BindEnvAndSetDefault("orchestrator_explorer.extra_tags", []string{})

	// Network
	config.BindEnv("network.id")

	// inventories
	config.BindEnvAndSetDefault("inventories_enabled", true)
	config.BindEnvAndSetDefault("inventories_max_interval", DefaultInventoriesMaxInterval) // integer seconds
	config.BindEnvAndSetDefault("inventories_min_interval", DefaultInventoriesMinInterval) // integer seconds

	// Datadog security agent (common)
	config.BindEnvAndSetDefault("security_agent.cmd_port", 5010)
	config.BindEnvAndSetDefault("security_agent.expvar_port", 5011)
	config.BindEnvAndSetDefault("security_agent.log_file", defaultSecurityAgentLogFile)
	config.BindEnvAndSetDefault("security_agent.remote_tagger", true)

	// Datadog security agent (compliance)
	config.BindEnvAndSetDefault("compliance_config.enabled", false)
	config.BindEnvAndSetDefault("compliance_config.check_interval", 20*time.Minute)
	config.BindEnvAndSetDefault("compliance_config.check_max_events_per_run", 100)
	config.BindEnvAndSetDefault("compliance_config.dir", "/etc/datadog-agent/compliance.d")
	config.BindEnvAndSetDefault("compliance_config.run_path", defaultRunPath)
	config.BindEnv("compliance_config.run_commands_as")
	bindEnvAndSetLogsConfigKeys(config, "compliance_config.endpoints.")

	// Datadog security agent (runtime)
	config.BindEnvAndSetDefault("runtime_security_config.enabled", false)
	config.SetKnown("runtime_security_config.fim_enabled")
	config.BindEnvAndSetDefault("runtime_security_config.erpc_dentry_resolution_enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.map_dentry_resolution_enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.dentry_cache_size", 1024)
	config.BindEnvAndSetDefault("runtime_security_config.policies.dir", DefaultRuntimePoliciesDir)
	config.BindEnvAndSetDefault("runtime_security_config.socket", "/opt/datadog-agent/run/runtime-security.sock")
	config.BindEnvAndSetDefault("runtime_security_config.enable_approvers", true)
	config.BindEnvAndSetDefault("runtime_security_config.enable_kernel_filters", true)
	config.BindEnvAndSetDefault("runtime_security_config.flush_discarder_window", 3)
	config.BindEnvAndSetDefault("runtime_security_config.syscall_monitor.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.runtime_monitor.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.events_stats.polling_interval", 20)
	config.BindEnvAndSetDefault("runtime_security_config.events_stats.tags_cardinality", "high")
	config.BindEnvAndSetDefault("runtime_security_config.run_path", defaultRunPath)
	config.BindEnvAndSetDefault("runtime_security_config.event_server.burst", 40)
	config.BindEnvAndSetDefault("runtime_security_config.event_server.retention", 6)
	config.BindEnvAndSetDefault("runtime_security_config.event_server.rate", 10)
	config.BindEnvAndSetDefault("runtime_security_config.load_controller.events_count_threshold", 20000)
	config.BindEnvAndSetDefault("runtime_security_config.load_controller.discarder_timeout", 60)
	config.BindEnvAndSetDefault("runtime_security_config.load_controller.control_period", 2)
	config.BindEnvAndSetDefault("runtime_security_config.pid_cache_size", 10000)
	config.BindEnvAndSetDefault("runtime_security_config.cookie_cache_size", 100)
	config.BindEnvAndSetDefault("runtime_security_config.agent_monitoring_events", true)
	config.BindEnvAndSetDefault("runtime_security_config.custom_sensitive_words", []string{})
	config.BindEnvAndSetDefault("runtime_security_config.remote_tagger", true)
	config.BindEnvAndSetDefault("runtime_security_config.log_patterns", []string{})
	config.BindEnvAndSetDefault("runtime_security_config.log_tags", []string{})
	bindEnvAndSetLogsConfigKeys(config, "runtime_security_config.endpoints.")
	config.BindEnvAndSetDefault("runtime_security_config.self_test.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.enable_remote_configuration", false)
	config.BindEnvAndSetDefault("runtime_security_config.runtime_compilation.enabled", false)
	config.BindEnv("runtime_security_config.runtime_compilation.compiled_constants_enabled")
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.cleanup_period", 30)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.tags_resolution_period", 60)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.traced_cgroups_count", -1)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.traced_event_types", []string{"exec", "open"})
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.cgroup_dump_timeout", 30)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.cgroup_wait_list_size", 10)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.cgroup_output_directory", "")
	config.BindEnvAndSetDefault("runtime_security_config.network.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.network.lazy_interface_prefixes", []string{})

	// Serverless Agent
	config.BindEnvAndSetDefault("serverless.logs_enabled", true)
	config.BindEnvAndSetDefault("enhanced_metrics", true)
	config.BindEnvAndSetDefault("capture_lambda_payload", false)

	// command line options
	config.SetKnown("cmd.check.fullsketches")

	// Vector integration
	bindVectorOptions(config, Metrics)
	bindVectorOptions(config, Logs)

	setAssetFs(config)
	setupAPM(config)
	setupAppSec(config)
	SetupOTLP(config)
	setupProcesses(config)
}

var ddURLRegexp = regexp.MustCompile(`^app(\.(us|eu)\d)?\.(datad(oghq|0g)\.(com|eu)|ddog-gov\.com)$`)

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
		if len(p.NoProxy) > 0 {
			config.Set("proxy.no_proxy", p.NoProxy)
		} else {
			// If this is set to an empty []string, viper will have a type conflict when merging
			// this config during secrets resolution. It unmarshals empty yaml lists to type
			// []interface{}, which will then conflict with type []string and fail to merge.
			config.Set("proxy.no_proxy", []interface{}{})
		}
		proxies = p
	}

	if !config.GetBool("use_proxy_for_cloud_metadata") {
		p.NoProxy = append(p.NoProxy, "169.254.169.254") // Azure, EC2, GCE
		p.NoProxy = append(p.NoProxy, "100.100.100.200") // Alibaba
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

func findUnknownEnvVars(config Config, environ []string) []string {
	var unknownVars []string

	knownVars := map[string]struct{}{
		// these variables are used by the agent, but not via the Config struct,
		// so must be listed separately.
		"DD_INSIDE_CI":      {},
		"DD_PROXY_NO_PROXY": {},
		"DD_PROXY_HTTP":     {},
		"DD_PROXY_HTTPS":    {},
		// these variables are used by serverless, but not via the Config struct
		"DD_SERVICE":            {},
		"DD_DOTNET_TRACER_HOME": {},
		"DD_TRACE_ENABLED":      {},
	}
	for _, key := range config.GetEnvVars() {
		knownVars[key] = struct{}{}
	}

	for _, equality := range environ {
		key := strings.SplitN(equality, "=", 2)[0]
		if !strings.HasPrefix(key, "DD_") {
			continue
		}
		if _, known := knownVars[key]; !known {
			unknownVars = append(unknownVars, key)
		}
	}
	return unknownVars
}

func useHostEtc(config Config) {
	if IsContainerized() && pathExists("/host/etc") {
		if !config.GetBool("ignore_host_etc") {
			if val, isSet := os.LookupEnv("HOST_ETC"); !isSet {
				// We want to detect the host distro informations instead of the one from the container.
				// 'HOST_ETC' is used by some libraries like gopsutil and by the system-probe to
				// download the right kernel headers.
				os.Setenv("HOST_ETC", "/host/etc")
				log.Debug("Setting environment variable HOST_ETC to '/host/etc'")
			} else {
				log.Debugf("'/host/etc' folder detected but HOST_ETC is already set to '%s', leaving it untouched", val)
			}
		} else {
			log.Debug("/host/etc detected but ignored because 'ignore_host_etc' is set to true")
		}
	}
}

func load(config Config, origin string, loadSecret bool) (*Warnings, error) {
	warnings := Warnings{}

	// Feature detection running in a defer func as it always  need to run (whether config load has been successful or not)
	// Because some Agents (e.g. trace-agent) will run even if config file does not exist
	defer func() {
		// Environment feature detection needs to run before applying override funcs
		// as it may provide such overrides
		DetectFeatures()
		applyOverrideFuncs(config)
	}()

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

	for _, v := range findUnknownEnvVars(config, os.Environ()) {
		log.Warnf("Unknown environment variable: %v", v)
	}

	if loadSecret {
		if err := ResolveSecrets(config, origin); err != nil {
			return &warnings, err
		}
	}

	// Verify 'DD_URL' and 'DD_DD_URL' conflicts
	if EnvVarAreSetAndNotEqual("DD_DD_URL", "DD_URL") {
		log.Warnf("'DD_URL' and 'DD_DD_URL' variables are both set in environment. Using 'DD_DD_URL' value")
	}

	// If this variable is set to true, we'll use DefaultPython for the Python version,
	// ignoring the python_version configuration value.
	if ForceDefaultPython == "true" {
		pv := config.GetString("python_version")
		if pv != DefaultPython {
			log.Warnf("Python version has been forced to %s", DefaultPython)
		}

		AddOverride("python_version", DefaultPython)
	}

	useHostEtc(config)
	loadProxyFromEnv(config)
	SanitizeAPIKeyConfig(config, "api_key")
	// setTracemallocEnabled *must* be called before setNumWorkers
	warnings.TraceMallocEnabledWithPy2 = setTracemallocEnabled(config)
	setNumWorkers(config)
	return &warnings, nil
}

// ResolveSecrets merges all the secret values from origin into config. Secret values
// are identified by a value of the form "ENC[key]" where key is the secret key.
// See: https://github.com/DataDog/datadog-agent/blob/main/docs/agent/secrets.md
func ResolveSecrets(config Config, origin string) error {
	// We have to init the secrets package before we can use it to decrypt
	// anything.
	secrets.Init(
		config.GetString("secret_backend_command"),
		config.GetStringSlice("secret_backend_arguments"),
		config.GetInt("secret_backend_timeout"),
		config.GetInt("secret_backend_output_max_size"),
		config.GetBool("secret_backend_command_allow_group_exec_perm"),
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

// EnvVarAreSetAndNotEqual returns true if two given variables are set in environment and are not equal.
func EnvVarAreSetAndNotEqual(lhsName string, rhsName string) bool {
	lhsValue, lhsIsSet := os.LookupEnv(lhsName)
	rhsValue, rhsIsSet := os.LookupEnv(rhsName)

	return lhsIsSet && rhsIsSet && lhsValue != rhsValue
}

// SanitizeAPIKeyConfig strips newlines and other control characters from a given key.
func SanitizeAPIKeyConfig(config Config, key string) {
	config.Set(key, SanitizeAPIKey(config.GetString(key)))
}

// sanitizeExternalMetricsProviderChunkSize ensures the value of `external_metrics_provider.chunk_size` is within an acceptable range
func sanitizeExternalMetricsProviderChunkSize(config Config) {
	chunkSize := config.GetInt("external_metrics_provider.chunk_size")
	if chunkSize <= 0 {
		log.Warnf("external_metrics_provider.chunk_size cannot be negative: %d", chunkSize)
		config.Set("external_metrics_provider.chunk_size", 1)
	}
	if chunkSize > maxExternalMetricsProviderChunkSize {
		log.Warnf("external_metrics_provider.chunk_size has been set to %d, which is higher than the maximum allowed value %d. Using %d.", chunkSize, maxExternalMetricsProviderChunkSize, maxExternalMetricsProviderChunkSize)
		config.Set("external_metrics_provider.chunk_size", maxExternalMetricsProviderChunkSize)
	}
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

func bindEnvAndSetLogsConfigKeys(config Config, prefix string) {
	config.BindEnv(prefix + "logs_dd_url") // Send the logs to a proxy. Must respect format '<HOST>:<PORT>' and '<PORT>' to be an integer
	config.BindEnv(prefix + "dd_url")
	config.BindEnv(prefix + "additional_endpoints")
	config.BindEnvAndSetDefault(prefix+"use_compression", true)
	config.BindEnvAndSetDefault(prefix+"compression_level", 6) // Default level for the gzip/deflate algorithm
	config.BindEnvAndSetDefault(prefix+"batch_wait", DefaultBatchWait)
	config.BindEnvAndSetDefault(prefix+"connection_reset_interval", 0) // in seconds, 0 means disabled
	config.BindEnvAndSetDefault(prefix+"logs_no_ssl", false)
	config.BindEnvAndSetDefault(prefix+"batch_max_concurrent_send", DefaultBatchMaxConcurrentSend)
	config.BindEnvAndSetDefault(prefix+"batch_max_content_size", DefaultBatchMaxContentSize)
	config.BindEnvAndSetDefault(prefix+"batch_max_size", DefaultBatchMaxSize)
	config.BindEnvAndSetDefault(prefix+"sender_backoff_factor", DefaultLogsSenderBackoffFactor)
	config.BindEnvAndSetDefault(prefix+"sender_backoff_base", DefaultLogsSenderBackoffBase)
	config.BindEnvAndSetDefault(prefix+"sender_backoff_max", DefaultLogsSenderBackoffMax)
	config.BindEnvAndSetDefault(prefix+"sender_recovery_interval", DefaultForwarderRecoveryInterval)
	config.BindEnvAndSetDefault(prefix+"sender_recovery_reset", false)
	config.BindEnvAndSetDefault(prefix+"use_v2_api", true)
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

	return MergeAdditionalEndpoints(keysPerDomain, additionalEndpoints)
}

// MergeAdditionalEndpoints merges additional endpoints into keysPerDomain
func MergeAdditionalEndpoints(keysPerDomain, additionalEndpoints map[string][]string) (map[string][]string, error) {
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
		if strings.EqualFold(cloudName, cloudProviderName) {
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

// pathExists returns true if the given path exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
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

	var cps []ConfigurationProviders
	if err := Datadog.UnmarshalKey("config_providers", &cps); err != nil {
		return false
	}

	for _, name := range Datadog.GetStringSlice("extra_config_providers") {
		cps = append(cps, ConfigurationProviders{Name: name})
	}

	// A cluster check runner is an Agent configured to run clusterchecks only
	// We want exactly one ConfigProvider named clusterchecks
	if len(cps) == 0 {
		return false
	}

	for _, cp := range cps {
		if cp.Name != "clusterchecks" {
			return false
		}
	}

	return true
}

// GetBindHost returns `bind_host` variable or default value
// Not using `config.BindEnvAndSetDefault` as some processes need to know
// if value was default one or not (e.g. trace-agent)
func GetBindHost() string {
	return getBindHost(Datadog)
}

func getBindHost(cfg Config) string {
	if cfg.IsSet("bind_host") {
		return cfg.GetString("bind_host")
	}
	return "localhost"
}

// GetValidHostAliases validates host aliases set in `host_aliases` variable and returns
// only valid ones.
func GetValidHostAliases() []string {
	return getValidHostAliasesWithConfig(Datadog)
}

func getValidHostAliasesWithConfig(config Config) []string {
	aliases := []string{}
	for _, alias := range config.GetStringSlice("host_aliases") {
		if err := validate.ValidHostname(alias); err == nil {
			aliases = append(aliases, alias)
		} else {
			log.Warnf("skipping invalid host alias '%s': %s", alias, err)
		}
	}

	return aliases
}

// GetConfiguredTags returns complete list of user configured tags.
//
// This is composed of DD_TAGS and DD_EXTRA_TAGS, with DD_DOGSTATSD_TAGS included
// if includeDogstatsd is true.
func GetConfiguredTags(includeDogstatsd bool) []string {
	tags := Datadog.GetStringSlice("tags")
	extraTags := Datadog.GetStringSlice("extra_tags")

	var dsdTags []string
	if includeDogstatsd {
		dsdTags = Datadog.GetStringSlice("dogstatsd_tags")
	}

	combined := make([]string, 0, len(tags)+len(extraTags)+len(dsdTags))
	combined = append(combined, tags...)
	combined = append(combined, extraTags...)
	combined = append(combined, dsdTags...)

	return combined
}

func bindVectorOptions(config Config, datatype DataType) {
	config.BindEnvAndSetDefault(fmt.Sprintf("vector.%s.enabled", datatype), false)
	config.BindEnvAndSetDefault(fmt.Sprintf("vector.%s.url", datatype), "")
}

// GetVectorURL returns the URL under the 'vector.' prefix for the given datatype
func GetVectorURL(datatype DataType) (string, error) {
	if Datadog.GetBool(fmt.Sprintf("vector.%s.enabled", datatype)) {
		vectorURL := Datadog.GetString(fmt.Sprintf("vector.%s.url", datatype))
		if vectorURL == "" {
			log.Errorf("vector.%s.enabled is set to true, but vector.%s.url is empty", datatype, datatype)
			return "", nil
		}
		_, err := url.Parse(vectorURL)
		if err != nil {
			return "", fmt.Errorf("could not parse vector %s endpoint: %s", datatype, err)
		}
		return vectorURL, nil
	}
	return "", nil
}

// GetInventoriesMinInterval gets the inventories_min_interval value, applying the default if it is zero.
func GetInventoriesMinInterval() time.Duration {
	minInterval := time.Duration(Datadog.GetInt("inventories_min_interval")) * time.Second
	if minInterval == 0 {
		minInterval = DefaultInventoriesMinInterval * time.Second
	}
	return minInterval
}

// GetInventoriesMaxInterval gets the inventories_max_interval value, applying the default if it is zero.
func GetInventoriesMaxInterval() time.Duration {
	maxInterval := time.Duration(Datadog.GetInt("inventories_max_interval")) * time.Second
	if maxInterval == 0 {
		maxInterval = DefaultInventoriesMaxInterval * time.Second
	}
	return maxInterval
}
