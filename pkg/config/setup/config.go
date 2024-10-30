// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package setup defines the configuration of the agent
package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
	"github.com/DataDog/datadog-agent/pkg/config/teeconfig"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

const (

	// DefaultSite is the default site the Agent sends data to.
	DefaultSite = "datadoghq.com"

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

	// DefaultInputChanSize is the default input chan size for events
	DefaultInputChanSize = 100

	// DefaultBatchMaxContentSize is the default HTTP batch max content size (before compression) for logs
	// It is also the maximum possible size of a single event. Events exceeding this limit are dropped.
	DefaultBatchMaxContentSize = 5000000

	// DefaultAuditorTTL is the default logs auditor TTL in hours
	DefaultAuditorTTL = 23

	// DefaultRuntimePoliciesDir is the default policies directory used by the runtime security module
	DefaultRuntimePoliciesDir = "/etc/datadog-agent/runtime-security.d"

	// DefaultCompressorKind is the default compressor. Options available are 'zlib' and 'zstd'
	DefaultCompressorKind = "zlib"

	// DefaultZstdCompressionLevel is the default compression level for `zstd`.
	// Compression level 1 provides the lowest compression ratio, but uses much less RSS especially
	// in situations where we have a high value for `GOMAXPROCS`.
	DefaultZstdCompressionLevel = 1

	// DefaultLogsSenderBackoffFactor is the default logs sender backoff randomness factor
	DefaultLogsSenderBackoffFactor = 2.0

	// DefaultLogsSenderBackoffBase is the default logs sender base backoff time, seconds
	DefaultLogsSenderBackoffBase = 1.0

	// DefaultLogsSenderBackoffMax is the default logs sender maximum backoff time, seconds
	DefaultLogsSenderBackoffMax = 120.0

	// DefaultLogsSenderBackoffRecoveryInterval is the default logs sender backoff recovery interval
	DefaultLogsSenderBackoffRecoveryInterval = 2

	// maxExternalMetricsProviderChunkSize ensures batch queries are limited in size.
	maxExternalMetricsProviderChunkSize = 35

	// DefaultLocalProcessCollectorInterval is the interval at which processes are collected and sent to the workloadmeta
	// in the core agent if the process check is disabled.
	DefaultLocalProcessCollectorInterval = 1 * time.Minute

	// DefaultMaxMessageSizeBytes is the default value for max_message_size_bytes
	// If a log message is larger than this byte limit, the overflow bytes will be truncated.
	DefaultMaxMessageSizeBytes = 256 * 1000

	// DefaultNetworkPathTimeout defines the default timeout for a network path test
	DefaultNetworkPathTimeout = 1000

	// DefaultNetworkPathMaxTTL defines the default maximum TTL for traceroute tests
	DefaultNetworkPathMaxTTL = 30
)

// datadog is the global configuration object
var (
	datadog     pkgconfigmodel.Config
	systemProbe pkgconfigmodel.Config

	datadogMutex     = sync.RWMutex{}
	systemProbeMutex = sync.RWMutex{}
)

// SetDatadog sets the the reference to the agent configuration.
// This is currently used by the legacy converter and config mocks and should not be user anywhere else. Once the
// legacy converter and mock have been migrated we will remove this function.
func SetDatadog(cfg pkgconfigmodel.Config) {
	datadogMutex.Lock()
	defer datadogMutex.Unlock()
	datadog = cfg
}

// SetSystemProbe sets the the reference to the systemProbe configuration.
// This is currently used by the config mocks and should not be user anywhere else. Once the mocks have been migrated we
// will remove this function.
func SetSystemProbe(cfg pkgconfigmodel.Config) {
	systemProbeMutex.Lock()
	defer systemProbeMutex.Unlock()
	systemProbe = cfg
}

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
)

// GetDefaultSecurityProfilesDir is the default directory used to store Security Profiles by the runtime security module
func GetDefaultSecurityProfilesDir() string {
	return filepath.Join(defaultRunPath, "runtime-security", "profiles")
}

// List of integrations allowed to be configured by RC by default
var defaultAllowedRCIntegrations = []string{}

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

// DataType represent the generic data type (e.g. metrics, logs) that can be sent by the Agent
type DataType string

const (
	// Metrics type covers series & sketches
	Metrics DataType = "metrics"
	// Logs type covers all outgoing logs
	Logs DataType = "logs"
)

// serverlessConfigComponents are the config components that are used by all agents, and in particular serverless.
// Components should only be added here if they are reachable by the serverless agent.
// Otherwise directly add the configs to InitConfig.
var serverlessConfigComponents = []func(pkgconfigmodel.Setup){
	agent,
	fips,
	dogstatsd,
	forwarder,
	aggregator,
	serializer,
	serverless,
	setupAPM,
	OTLP,
	setupMultiRegionFailover,
	telemetry,
	autoconfig,
	remoteconfig,
	logsagent,
	containerSyspath,
	containerd,
	cri,
	kubernetes,
	cloudfoundry,
	debugging,
	vector,
	podman,
}

func init() {
	osinit()

	// Configure Datadog global configuration
	envvar, found := os.LookupEnv("DD_CONF_NODETREEMODEL")
	// Possible values for DD_CONF_NODETREEMODEL:
	// - "enable": Use the nodetreemodel for the config, instead of viper
	// - "tee":    Construct both viper and nodetreemodel. Write to both, only read from viper
	// - other:    Use viper for the config
	if found && envvar == "enable" {
		datadog = nodetreemodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
	} else if found && envvar == "tee" {
		var viperConfig = pkgconfigmodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))   // nolint: forbidigo // legit use case
		var nodetreeConfig = nodetreemodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
		datadog = teeconfig.NewTeeConfig(viperConfig, nodetreeConfig)
	} else {
		datadog = pkgconfigmodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
	}

	systemProbe = pkgconfigmodel.NewConfig("system-probe", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case

	// Configuration defaults
	initConfig()
}

// initCommonWithServerless initializes configs that are common to all agents, in particular serverless.
// Initializing the config keys takes too much time for serverless, so we try to initialize only what is reachable.
func initCommonWithServerless(config pkgconfigmodel.Setup) {
	for _, f := range serverlessConfigComponents {
		f(config)
	}
}

// InitConfig initializes the config defaults on a config used by all agents
// (in particular more than just the serverless agent).
func InitConfig(config pkgconfigmodel.Setup) {
	initCommonWithServerless(config)

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
	config.BindEnvAndSetDefault("check_sampler_context_metrics", false)
	config.BindEnvAndSetDefault("host_aliases", []string{})

	// overridden in IoT Agent main
	config.BindEnvAndSetDefault("iot_host", false)
	// overridden in Heroku buildpack
	config.BindEnvAndSetDefault("heroku_dyno", false)

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

	// By default the Agent does not trust the hostname value retrieved from non-root UTS namespace.
	// When enabled, the Agent will trust the value retrieved from non-root UTS namespace instead of failing
	// hostname resolution.
	// (Linux only)
	config.BindEnvAndSetDefault("hostname_trust_uts_namespace", false)

	config.BindEnvAndSetDefault("cluster_name", "")
	config.BindEnvAndSetDefault("disable_cluster_name_tag_key", false)
	config.BindEnvAndSetDefault("enabled_rfc1123_compliant_cluster_name_tag", true)

	// secrets backend
	config.BindEnvAndSetDefault("secret_backend_command", "")
	config.BindEnvAndSetDefault("secret_backend_arguments", []string{})
	config.BindEnvAndSetDefault("secret_backend_output_max_size", 0)
	config.BindEnvAndSetDefault("secret_backend_timeout", 0)
	config.BindEnvAndSetDefault("secret_backend_command_allow_group_exec_perm", false)
	config.BindEnvAndSetDefault("secret_backend_skip_checks", false)
	config.BindEnvAndSetDefault("secret_backend_remove_trailing_line_break", false)
	config.BindEnvAndSetDefault("secret_refresh_interval", 0)
	config.SetDefault("secret_audit_file_max_size", 0)

	// IPC API server timeout
	config.BindEnvAndSetDefault("server_timeout", 30)

	// Defaults to safe YAML methods in base and custom checks.
	config.BindEnvAndSetDefault("disable_unsafe_yaml", true)

	// Yaml keys which values are stripped from flare
	config.BindEnvAndSetDefault("flare_stripped_keys", []string{})
	config.BindEnvAndSetDefault("scrubber.additional_keys", []string{})

	// flare configs
	config.BindEnvAndSetDefault("flare_provider_timeout", 10*time.Second)

	// Docker
	config.BindEnvAndSetDefault("docker_query_timeout", int64(5))
	config.BindEnvAndSetDefault("docker_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("docker_env_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_pod_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_pod_annotations_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_node_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_node_annotations_as_tags", map[string]string{"cluster.k8s.io/machine": "kube_machine"})
	config.BindEnvAndSetDefault("kubernetes_node_annotations_as_host_aliases", []string{"cluster.k8s.io/machine"})
	config.BindEnvAndSetDefault("kubernetes_node_label_as_cluster_name", "")
	config.BindEnvAndSetDefault("kubernetes_namespace_labels_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("kubernetes_namespace_annotations_as_tags", map[string]string{})
	// kubernetes_resources_annotations_as_tags should be parseable as map[string]map[string]string
	// it maps group resources to annotations as tags maps
	// a group resource has the format `{resource}.{group}`, or simply `{resource}` if it belongs to the empty group
	// examples of group resources:
	// 	- `deployments.apps`
	// 	- `statefulsets.apps`
	// 	- `pods`
	// 	- `nodes`
	config.BindEnvAndSetDefault("kubernetes_resources_annotations_as_tags", "{}")
	// kubernetes_resources_labels_as_tags should be parseable as map[string]map[string]string
	// it maps group resources to labels as tags maps
	// a group resource has the format `{resource}.{group}`, or simply `{resource}` if it belongs to the empty group
	// examples of group resources:
	// 	- `deployments.apps`
	// 	- `statefulsets.apps`
	// 	- `pods`
	// 	- `nodes`
	config.BindEnvAndSetDefault("kubernetes_resources_labels_as_tags", "{}")
	config.BindEnvAndSetDefault("kubernetes_persistent_volume_claims_as_tags", true)
	config.BindEnvAndSetDefault("container_cgroup_prefix", "")

	config.BindEnvAndSetDefault("prometheus_scrape.enabled", false)           // Enables the prometheus config provider
	config.BindEnvAndSetDefault("prometheus_scrape.service_endpoints", false) // Enables Service Endpoints checks in the prometheus config provider
	config.BindEnv("prometheus_scrape.checks")                                // Defines any extra prometheus/openmetrics check configurations to be handled by the prometheus config provider
	config.BindEnvAndSetDefault("prometheus_scrape.version", 1)               // Version of the openmetrics check to be scheduled by the Prometheus auto-discovery

	// Network Devices Monitoring
	bindEnvAndSetLogsConfigKeys(config, "network_devices.metadata.")
	config.BindEnvAndSetDefault("network_devices.namespace", "default")

	config.SetKnown("snmp_listener.discovery_interval")
	config.SetKnown("snmp_listener.allowed_failures")
	config.SetKnown("snmp_listener.discovery_allowed_failures")
	config.SetKnown("snmp_listener.collect_device_metadata")
	config.SetKnown("snmp_listener.collect_topology")
	config.SetKnown("snmp_listener.workers")
	config.SetKnown("snmp_listener.configs")
	config.SetKnown("snmp_listener.loader")
	config.SetKnown("snmp_listener.min_collection_interval")
	config.SetKnown("snmp_listener.namespace")
	config.SetKnown("snmp_listener.use_device_id_as_hostname")
	config.SetKnown("snmp_listener.ping")
	config.SetKnown("snmp_listener.ping.enabled")
	config.SetKnown("snmp_listener.ping.count")
	config.SetKnown("snmp_listener.ping.interval")
	config.SetKnown("snmp_listener.ping.timeout")
	config.SetKnown("snmp_listener.ping.linux")
	config.SetKnown("snmp_listener.ping.linux.use_raw_socket")

	// network_devices.autodiscovery has precedence over snmp_listener config
	// snmp_listener config is still here for legacy reasons
	config.SetKnown("network_devices.autodiscovery.discovery_interval")
	config.SetKnown("network_devices.autodiscovery.allowed_failures")
	config.SetKnown("network_devices.autodiscovery.discovery_allowed_failures")
	config.SetKnown("network_devices.autodiscovery.collect_device_metadata")
	config.SetKnown("network_devices.autodiscovery.collect_topology")
	config.SetKnown("network_devices.autodiscovery.workers")
	config.SetKnown("network_devices.autodiscovery.configs")
	config.SetKnown("network_devices.autodiscovery.loader")
	config.SetKnown("network_devices.autodiscovery.min_collection_interval")
	config.SetKnown("network_devices.autodiscovery.namespace")
	config.SetKnown("network_devices.autodiscovery.use_device_id_as_hostname")
	config.SetKnown("network_devices.autodiscovery.ping")
	config.SetKnown("network_devices.autodiscovery.ping.enabled")
	config.SetKnown("network_devices.autodiscovery.ping.count")
	config.SetKnown("network_devices.autodiscovery.ping.interval")
	config.SetKnown("network_devices.autodiscovery.ping.timeout")
	config.SetKnown("network_devices.autodiscovery.ping.linux")
	config.SetKnown("network_devices.autodiscovery.ping.linux.use_raw_socket")

	bindEnvAndSetLogsConfigKeys(config, "network_devices.snmp_traps.forwarder.")
	config.BindEnvAndSetDefault("network_devices.snmp_traps.enabled", false)
	config.BindEnvAndSetDefault("network_devices.snmp_traps.port", 9162)
	config.BindEnvAndSetDefault("network_devices.snmp_traps.community_strings", []string{})
	config.BindEnvAndSetDefault("network_devices.snmp_traps.bind_host", "0.0.0.0")
	config.BindEnvAndSetDefault("network_devices.snmp_traps.stop_timeout", 5) // in seconds
	config.SetKnown("network_devices.snmp_traps.users")

	// NetFlow
	config.SetKnown("network_devices.netflow.listeners")
	config.SetKnown("network_devices.netflow.stop_timeout")
	config.SetKnown("network_devices.netflow.aggregator_buffer_size")
	config.SetKnown("network_devices.netflow.aggregator_flush_interval")
	config.SetKnown("network_devices.netflow.aggregator_flow_context_ttl")
	config.SetKnown("network_devices.netflow.aggregator_port_rollup_threshold")
	config.SetKnown("network_devices.netflow.aggregator_rollup_tracker_refresh_interval")
	config.BindEnvAndSetDefault("network_devices.netflow.enabled", "false")
	bindEnvAndSetLogsConfigKeys(config, "network_devices.netflow.forwarder.")
	config.BindEnvAndSetDefault("network_devices.netflow.reverse_dns_enrichment_enabled", false)

	// Network Path
	config.BindEnvAndSetDefault("network_path.connections_monitoring.enabled", false)
	config.BindEnvAndSetDefault("network_path.collector.workers", 4)
	config.BindEnvAndSetDefault("network_path.collector.timeout", DefaultNetworkPathTimeout)
	config.BindEnvAndSetDefault("network_path.collector.max_ttl", DefaultNetworkPathMaxTTL)
	config.BindEnvAndSetDefault("network_path.collector.input_chan_size", 100000)
	config.BindEnvAndSetDefault("network_path.collector.processing_chan_size", 100000)
	config.BindEnvAndSetDefault("network_path.collector.pathtest_contexts_limit", 100000)
	config.BindEnvAndSetDefault("network_path.collector.pathtest_ttl", "15m")
	config.BindEnvAndSetDefault("network_path.collector.pathtest_interval", "5m")
	config.BindEnvAndSetDefault("network_path.collector.flush_interval", "10s")
	bindEnvAndSetLogsConfigKeys(config, "network_path.forwarder.")

	// Kube ApiServer
	config.BindEnvAndSetDefault("kubernetes_kubeconfig_path", "")
	config.BindEnvAndSetDefault("kubernetes_apiserver_ca_path", "")
	config.BindEnvAndSetDefault("kubernetes_apiserver_tls_verify", true)
	config.BindEnvAndSetDefault("leader_lease_duration", "60")
	config.BindEnvAndSetDefault("leader_election", false)
	config.BindEnvAndSetDefault("leader_lease_name", "datadog-leader-election")
	config.BindEnvAndSetDefault("leader_election_default_resource", "configmap")
	config.BindEnvAndSetDefault("leader_election_release_on_shutdown", true)
	config.BindEnvAndSetDefault("kube_resources_namespace", "")
	config.BindEnvAndSetDefault("kube_cache_sync_timeout_seconds", 5)

	// Datadog cluster agent
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
	config.BindEnvAndSetDefault("metrics_port", "5000")
	config.BindEnvAndSetDefault("cluster_agent.language_detection.patcher.enabled", true)
	config.BindEnvAndSetDefault("cluster_agent.language_detection.patcher.base_backoff", "5m")
	config.BindEnvAndSetDefault("cluster_agent.language_detection.patcher.max_backoff", "1h")
	// sets the expiration deadline (TTL) for reported languages
	config.BindEnvAndSetDefault("cluster_agent.language_detection.cleanup.language_ttl", "30m")
	// language annotation cleanup period
	config.BindEnvAndSetDefault("cluster_agent.language_detection.cleanup.period", "10m")
	config.BindEnvAndSetDefault("cluster_agent.kube_metadata_collection.enabled", false)
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
	config.BindEnvAndSetDefault("cluster_agent.cluster_tagger.grpc_max_message_size", 4<<20) // 4 MB

	// Metadata endpoints

	// Defines the maximum size of hostame gathered from EC2, GCE, Azure, Alibaba, Oracle and Tencent cloud metadata
	// endpoints (all cloudprovider except IBM). IBM cloud ignore this setting as their API return a huge JSON with
	// all the metadata for the VM.
	// Used internally to protect against configurations where metadata endpoints return incorrect values with 200 status codes.
	config.BindEnvAndSetDefault("metadata_endpoints_max_hostname_size", 255)

	// Duration during which the host tags will be submitted with metrics.
	config.BindEnvAndSetDefault("expected_tags_duration", time.Duration(0))

	// EC2
	config.BindEnvAndSetDefault("ec2_use_windows_prefix_detection", false)
	config.BindEnvAndSetDefault("ec2_metadata_timeout", 300)          // value in milliseconds
	config.BindEnvAndSetDefault("ec2_metadata_token_lifetime", 21600) // value in seconds
	config.BindEnvAndSetDefault("ec2_prefer_imdsv2", false)
	config.BindEnvAndSetDefault("ec2_prioritize_instance_id_as_hostname", false) // used to bypass the hostname detection logic and force the EC2 instance ID as a hostname.
	config.BindEnvAndSetDefault("ec2_use_dmi", true)                             // should the agent leverage DMI information to know if it's running on EC2 or not. Enabling this will add the instance ID from DMI to the host alias list.
	config.BindEnvAndSetDefault("collect_ec2_tags", false)
	config.BindEnvAndSetDefault("collect_ec2_tags_use_imds", false)
	config.BindEnvAndSetDefault("exclude_ec2_tags", []string{})

	// ECS
	config.BindEnvAndSetDefault("ecs_agent_url", "") // Will be autodetected
	config.BindEnvAndSetDefault("ecs_agent_container_name", "ecs-agent")
	config.BindEnvAndSetDefault("ecs_collect_resource_tags_ec2", false)
	config.BindEnvAndSetDefault("ecs_resource_tags_replace_colon", false)
	config.BindEnvAndSetDefault("ecs_metadata_timeout", 500) // value in milliseconds
	config.BindEnvAndSetDefault("ecs_metadata_retry_initial_interval", 100*time.Millisecond)
	config.BindEnvAndSetDefault("ecs_metadata_retry_max_elapsed_time", 3000*time.Millisecond)
	config.BindEnvAndSetDefault("ecs_metadata_retry_timeout_factor", 3)
	config.BindEnvAndSetDefault("ecs_task_collection_enabled", false)
	config.BindEnvAndSetDefault("ecs_task_cache_ttl", 3*time.Minute)
	config.BindEnvAndSetDefault("ecs_task_collection_rate", 35)
	config.BindEnvAndSetDefault("ecs_task_collection_burst", 60)

	// GCE
	config.BindEnvAndSetDefault("collect_gce_tags", true)
	config.BindEnvAndSetDefault("exclude_gce_tags", []string{
		"kube-env", "kubelet-config", "containerd-configure-sh", "startup-script", "shutdown-script",
		"configure-sh", "sshKeys", "ssh-keys", "user-data", "cli-cert", "ipsec-cert", "ssl-cert", "google-container-manifest",
		"bosh_settings", "windows-startup-script-ps1", "common-psm1", "k8s-node-setup-psm1", "serial-port-logging-enable",
		"enable-oslogin", "disable-address-manager", "disable-legacy-endpoints", "windows-keys", "kubeconfig", "gce-container-declaration",
	})
	config.BindEnvAndSetDefault("gce_send_project_id_tag", false)
	config.BindEnvAndSetDefault("gce_metadata_timeout", 1000) // value in milliseconds

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

	// Cloud Foundry Container Tagger
	config.BindEnvAndSetDefault("cloud_foundry_container_tagger.shell_path", "/bin/sh")
	config.BindEnvAndSetDefault("cloud_foundry_container_tagger.retry_count", 10)
	config.BindEnvAndSetDefault("cloud_foundry_container_tagger.retry_interval", 10)

	// Azure
	config.BindEnvAndSetDefault("azure_hostname_style", "os")

	// IBM cloud
	// We use a long timeout here since the metadata and token API can be very slow sometimes.
	config.BindEnvAndSetDefault("ibm_metadata_timeout", 5) // value in seconds

	// JMXFetch
	config.BindEnvAndSetDefault("jmx_custom_jars", []string{})
	config.BindEnvAndSetDefault("jmx_use_cgroup_memory_limit", false)
	config.BindEnvAndSetDefault("jmx_use_container_support", false)
	config.BindEnvAndSetDefault("jmx_max_ram_percentage", float64(25.0))
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
	config.BindEnvAndSetDefault("expvar_port", "5000")

	// internal profiling
	config.BindEnvAndSetDefault("internal_profiling.enabled", false)
	config.BindEnv("internal_profiling.profile_dd_url")
	config.BindEnvAndSetDefault("internal_profiling.unix_socket", "") // file system path to a unix socket, e.g. `/var/run/datadog/apm.socket`
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

	// The cardinality of tags to send for checks and dogstatsd respectively.
	// Choices are: low, orchestrator, high.
	// WARNING: sending orchestrator, or high tags for dogstatsd metrics may create more metrics
	// (one per container instead of one per host).
	// Changing this setting may impact your custom metrics billing.
	config.BindEnvAndSetDefault("checks_tag_cardinality", "low")
	config.BindEnvAndSetDefault("dogstatsd_tag_cardinality", "low")

	// Autoscaling product
	config.BindEnvAndSetDefault("autoscaling.workload.enabled", false)

	config.BindEnvAndSetDefault("hpa_watcher_polling_freq", 10)
	config.BindEnvAndSetDefault("hpa_watcher_gc_period", 60*5) // 5 minutes
	config.BindEnvAndSetDefault("hpa_configmap_name", "datadog-custom-metrics")
	config.BindEnvAndSetDefault("external_metrics_provider.enabled", false)
	config.BindEnvAndSetDefault("external_metrics_provider.port", 8443)
	config.BindEnvAndSetDefault("external_metrics_provider.endpoint", "")                       // Override the Datadog API endpoint to query external metrics from
	config.BindEnvAndSetDefault("external_metrics_provider.api_key", "")                        // Override the Datadog API Key for external metrics endpoint
	config.BindEnvAndSetDefault("external_metrics_provider.app_key", "")                        // Override the Datadog APP Key for external metrics endpoint
	config.SetKnown("external_metrics_provider.endpoints")                                      // List of redundant endpoints to query external metrics from
	config.BindEnvAndSetDefault("external_metrics_provider.refresh_period", 30)                 // value in seconds. Frequency of calls to Datadog to refresh metric values
	config.BindEnvAndSetDefault("external_metrics_provider.batch_window", 10)                   // value in seconds. Batch the events from the Autoscalers informer to push updates to the ConfigMap (GlobalStore)
	config.BindEnvAndSetDefault("external_metrics_provider.max_age", 120)                       // value in seconds. 4 cycles from the Autoscaler controller (up to Kubernetes 1.11) is enough to consider a metric stale
	config.BindEnvAndSetDefault("external_metrics_provider.query_validity_period", 30)          // value in seconds. Represents grace period to account for delay between metric is resolved and when autoscaling controllers query for it
	config.BindEnvAndSetDefault("external_metrics.aggregator", "avg")                           // aggregator used for the external metrics. Choose from [avg,sum,max,min]
	config.BindEnvAndSetDefault("external_metrics_provider.max_time_window", 60*60*24)          // Maximum window to query to get the metric from Datadog.
	config.BindEnvAndSetDefault("external_metrics_provider.bucket_size", 60*5)                  // Window to query to get the metric from Datadog.
	config.BindEnvAndSetDefault("external_metrics_provider.rollup", 30)                         // Bucket size to circumvent time aggregation side effects.
	config.BindEnvAndSetDefault("external_metrics_provider.wpa_controller", false)              // Activates the controller for Watermark Pod Autoscalers.
	config.BindEnvAndSetDefault("external_metrics_provider.use_datadogmetric_crd", false)       // Use DatadogMetric CRD with custom Datadog Queries instead of ConfigMap
	config.BindEnvAndSetDefault("external_metrics_provider.enable_datadogmetric_autogen", true) // Enables autogeneration of DatadogMetrics when the DatadogMetric CRD is in use
	config.BindEnvAndSetDefault("kubernetes_event_collection_timeout", 100)                     // timeout between two successful event collections in milliseconds.
	config.BindEnvAndSetDefault("kubernetes_informers_resync_period", 60*5)                     // value in seconds. Default to 5 minutes
	config.BindEnvAndSetDefault("external_metrics_provider.config", map[string]string{})        // list of options that can be used to configure the external metrics server
	config.BindEnvAndSetDefault("external_metrics_provider.local_copy_refresh_rate", 30)        // value in seconds
	config.BindEnvAndSetDefault("external_metrics_provider.chunk_size", 35)                     // Maximum number of queries to batch when querying Datadog.
	config.BindEnvAndSetDefault("external_metrics_provider.split_batches_with_backoff", false)  // Splits batches and runs queries with errors individually with an exponential backoff
	pkgconfigmodel.AddOverrideFunc(sanitizeExternalMetricsProviderChunkSize)
	// Cluster check Autodiscovery
	config.BindEnvAndSetDefault("cluster_checks.support_hybrid_ignore_ad_tags", false) // TODO(CINT)(Agent 7.53+) Remove this flag when hybrid ignore_ad_tags is fully deprecated
	config.BindEnvAndSetDefault("cluster_checks.enabled", false)
	config.BindEnvAndSetDefault("cluster_checks.node_expiration_timeout", 30) // value in seconds
	config.BindEnvAndSetDefault("cluster_checks.warmup_duration", 30)         // value in seconds
	config.BindEnvAndSetDefault("cluster_checks.cluster_tag_name", "cluster_name")
	config.BindEnvAndSetDefault("cluster_checks.extra_tags", []string{})
	config.BindEnvAndSetDefault("cluster_checks.advanced_dispatching_enabled", false)
	config.BindEnvAndSetDefault("cluster_checks.rebalance_with_utilization", false)        // Experimental. Subject to change. Uses the runners utilization to balance.
	config.BindEnvAndSetDefault("cluster_checks.rebalance_min_percentage_improvement", 10) // Experimental. Subject to change. Rebalance only if the distribution found improves the current one by this.
	config.BindEnvAndSetDefault("cluster_checks.clc_runners_port", 5005)
	config.BindEnvAndSetDefault("cluster_checks.exclude_checks", []string{})
	config.BindEnvAndSetDefault("cluster_checks.exclude_checks_from_dispatching", []string{})
	config.BindEnvAndSetDefault("cluster_checks.rebalance_period", 10*time.Minute)

	// Cluster check runner
	config.BindEnvAndSetDefault("clc_runner_enabled", false)
	config.BindEnvAndSetDefault("clc_runner_id", "")
	config.BindEnvAndSetDefault("clc_runner_host", "") // must be set using the Kubernetes downward API
	config.BindEnvAndSetDefault("clc_runner_port", 5005)
	config.BindEnvAndSetDefault("clc_runner_server_write_timeout", 15)
	config.BindEnvAndSetDefault("clc_runner_server_readheader_timeout", 10)
	config.BindEnvAndSetDefault("clc_runner_remote_tagger_enabled", false)

	// Admission controller
	config.BindEnvAndSetDefault("admission_controller.enabled", false)
	config.BindEnvAndSetDefault("admission_controller.validation.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.mutation.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.mutate_unlabelled", false)
	config.BindEnvAndSetDefault("admission_controller.port", 8000)
	config.BindEnvAndSetDefault("admission_controller.container_registry", "gcr.io/datadoghq")
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
	config.BindEnvAndSetDefault("admission_controller.inject_config.trace_agent_socket", "unix:///var/run/datadog/apm.socket")
	config.BindEnvAndSetDefault("admission_controller.inject_config.dogstatsd_socket", "unix:///var/run/datadog/dsd.socket")
	config.BindEnvAndSetDefault("admission_controller.inject_config.type_socket_volumes", false)
	config.BindEnvAndSetDefault("admission_controller.inject_tags.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.inject_tags.endpoint", "/injecttags")
	config.BindEnvAndSetDefault("admission_controller.inject_tags.pod_owners_cache_validity", 10) // in minutes
	config.BindEnv("admission_controller.pod_owners_cache_validity")                              // Alias for admission_controller.inject_tags.pod_owners_cache_validity. Was added without the "inject_tags" prefix by mistake but needs to be kept for backwards compatibility
	config.BindEnvAndSetDefault("admission_controller.namespace_selector_fallback", false)
	config.BindEnvAndSetDefault("admission_controller.failure_policy", "Ignore")
	config.BindEnvAndSetDefault("admission_controller.reinvocation_policy", "IfNeeded")
	config.BindEnvAndSetDefault("admission_controller.add_aks_selectors", false) // adds in the webhook some selectors that are required in AKS
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.enabled", true)
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.endpoint", "/injectlib")
	config.BindEnv("admission_controller.auto_instrumentation.container_registry")
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.patcher.enabled", false)
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.patcher.fallback_to_file_provider", false)                                // to be enabled only in e2e tests
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.patcher.file_provider_path", "/etc/datadog-agent/patch/auto-instru.json") // to be used only in e2e tests
	config.BindEnvAndSetDefault("admission_controller.auto_instrumentation.inject_auto_detected_libraries", false)                                   // allows injecting libraries for languages detected by automatic language detection feature
	config.BindEnv("admission_controller.auto_instrumentation.init_resources.cpu")
	config.BindEnv("admission_controller.auto_instrumentation.init_resources.memory")
	config.BindEnv("admission_controller.auto_instrumentation.init_security_context")
	config.BindEnv("admission_controller.auto_instrumentation.asm.enabled", "DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_APPSEC_ENABLED")          // config for ASM which is implemented in the client libraries
	config.BindEnv("admission_controller.auto_instrumentation.iast.enabled", "DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_IAST_ENABLED")           // config for IAST which is implemented in the client libraries
	config.BindEnv("admission_controller.auto_instrumentation.asm_sca.enabled", "DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_APPSEC_SCA_ENABLED")  // config for SCA
	config.BindEnv("admission_controller.auto_instrumentation.profiling.enabled", "DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_PROFILING_ENABLED") // config for profiling
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.enabled", false)
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.pod_endpoint", "/inject-pod-cws")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.command_endpoint", "/inject-command-cws")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.include", []string{})
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.exclude", []string{})
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.mutate_unlabelled", false)
	config.BindEnv("admission_controller.cws_instrumentation.container_registry")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.image_name", "cws-instrumentation")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.image_tag", "latest")
	config.BindEnv("admission_controller.cws_instrumentation.init_resources.cpu")
	config.BindEnv("admission_controller.cws_instrumentation.init_resources.memory")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.mode", "remote_copy")
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.remote_copy.mount_volume", false)
	config.BindEnvAndSetDefault("admission_controller.cws_instrumentation.remote_copy.directory", "/tmp")
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.enabled", false)
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.provider", "")
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.endpoint", "/agentsidecar")
	// Should be able to parse it to a list of webhook selectors
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.selectors", "[]")
	// Should be able to parse it to a list of env vars and resource limits
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.profiles", "[]")
	config.BindEnv("admission_controller.agent_sidecar.container_registry")
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.image_name", "agent")
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.image_tag", "latest")
	config.BindEnvAndSetDefault("admission_controller.agent_sidecar.cluster_agent.enabled", "true")

	// Declare other keys that don't have a default/env var.
	// Mostly, keys we use IsSet() on, because IsSet always returns true if a key has a default.
	config.SetKnown("metadata_providers")
	config.SetKnown("config_providers")
	config.SetKnown("cluster_name")
	config.SetKnown("listeners")

	// Orchestrator Explorer DCA and core agent
	config.BindEnvAndSetDefault("orchestrator_explorer.enabled", true)
	// enabling/disabling the environment variables & command scrubbing from the container specs
	// this option will potentially impact the CPU usage of the agent
	config.BindEnvAndSetDefault("orchestrator_explorer.container_scrubbing.enabled", true)
	config.BindEnvAndSetDefault("orchestrator_explorer.custom_sensitive_words", []string{})
	config.BindEnvAndSetDefault("orchestrator_explorer.custom_sensitive_annotations_labels", []string{})
	config.BindEnvAndSetDefault("orchestrator_explorer.collector_discovery.enabled", true)
	config.BindEnv("orchestrator_explorer.max_per_message")
	config.BindEnv("orchestrator_explorer.max_message_bytes")
	config.BindEnv("orchestrator_explorer.orchestrator_dd_url", "DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL", "DD_ORCHESTRATOR_URL")
	config.BindEnv("orchestrator_explorer.orchestrator_additional_endpoints", "DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_ADDITIONAL_ENDPOINTS", "DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS")
	config.BindEnv("orchestrator_explorer.use_legacy_endpoint")
	config.BindEnvAndSetDefault("orchestrator_explorer.manifest_collection.enabled", true)
	config.BindEnvAndSetDefault("orchestrator_explorer.manifest_collection.buffer_manifest", true)
	config.BindEnvAndSetDefault("orchestrator_explorer.manifest_collection.buffer_flush_interval", 20*time.Second)

	// Container lifecycle configuration
	config.BindEnvAndSetDefault("container_lifecycle.enabled", true)
	bindEnvAndSetLogsConfigKeys(config, "container_lifecycle.")

	// Container image configuration
	config.BindEnvAndSetDefault("container_image.enabled", true)
	bindEnvAndSetLogsConfigKeys(config, "container_image.")

	// Remote process collector
	config.BindEnvAndSetDefault("workloadmeta.local_process_collector.collection_interval", DefaultLocalProcessCollectorInterval)

	// Tagger Component
	// This is a temporary/transient flag used to slowly migrate to a new internal implementation of the tagger.
	// If set to true, the tagger will store all entities in a 2-layered map, the first map is indexed by prefix, and the second one is indexed by id.
	// If set to false, the tagger will use the default implementation by storing entities in a one-layer map from plain strings to Tag Entities.
	// TODO: remove this config option when the migration is finalised.
	config.BindEnvAndSetDefault("tagger.tagstore_use_composite_entity_id", false)

	// SBOM configuration
	config.BindEnvAndSetDefault("sbom.enabled", false)
	bindEnvAndSetLogsConfigKeys(config, "sbom.")

	config.BindEnvAndSetDefault("sbom.cache_directory", filepath.Join(defaultRunPath, "sbom-agent"))
	config.BindEnvAndSetDefault("sbom.clear_cache_on_exit", false)
	config.BindEnvAndSetDefault("sbom.cache.max_disk_size", 1000*1000*100) // used by custom cache: max disk space used by cached objects. Not equal to max disk usage
	config.BindEnvAndSetDefault("sbom.cache.clean_interval", "1h")         // used by custom cache.
	config.BindEnvAndSetDefault("sbom.scan_queue.base_backoff", "5m")
	config.BindEnvAndSetDefault("sbom.scan_queue.max_backoff", "1h")

	// Container SBOM configuration
	config.BindEnvAndSetDefault("sbom.container_image.enabled", false)
	config.BindEnvAndSetDefault("sbom.container_image.use_mount", false)
	config.BindEnvAndSetDefault("sbom.container_image.scan_interval", 0)    // Integer seconds
	config.BindEnvAndSetDefault("sbom.container_image.scan_timeout", 10*60) // Integer seconds
	config.BindEnvAndSetDefault("sbom.container_image.analyzers", []string{"os"})
	config.BindEnvAndSetDefault("sbom.container_image.check_disk_usage", true)
	config.BindEnvAndSetDefault("sbom.container_image.min_available_disk", "1Gb")
	config.BindEnvAndSetDefault("sbom.container_image.overlayfs_direct_scan", false)

	// Host SBOM configuration
	config.BindEnvAndSetDefault("sbom.host.enabled", false)
	config.BindEnvAndSetDefault("sbom.host.analyzers", []string{"os"})

	// Service discovery configuration
	bindEnvAndSetLogsConfigKeys(config, "service_discovery.forwarder.")

	// Orchestrator Explorer - process agent
	// DEPRECATED in favor of `orchestrator_explorer.orchestrator_dd_url` setting. If both are set `orchestrator_explorer.orchestrator_dd_url` will take precedence.
	config.BindEnv("process_config.orchestrator_dd_url", "DD_PROCESS_CONFIG_ORCHESTRATOR_DD_URL", "DD_PROCESS_AGENT_ORCHESTRATOR_DD_URL")
	// DEPRECATED in favor of `orchestrator_explorer.orchestrator_additional_endpoints` setting. If both are set `orchestrator_explorer.orchestrator_additional_endpoints` will take precedence.
	config.SetKnown("process_config.orchestrator_additional_endpoints.*")
	config.SetKnown("orchestrator_explorer.orchestrator_additional_endpoints.*")
	config.BindEnvAndSetDefault("orchestrator_explorer.extra_tags", []string{})

	// Network
	config.BindEnv("network.id")

	// OTel Collector
	config.BindEnvAndSetDefault("otelcollector.enabled", false)
	config.BindEnvAndSetDefault("otelcollector.extension_url", "https://localhost:7777")
	config.BindEnvAndSetDefault("otelcollector.extension_timeout", 0)         // in seconds, 0 for default value
	config.BindEnvAndSetDefault("otelcollector.submit_dummy_metadata", false) // dev flag - to be removed
	config.BindEnvAndSetDefault("otelcollector.converter.enabled", true)

	// inventories
	config.BindEnvAndSetDefault("inventories_enabled", true)
	config.BindEnvAndSetDefault("inventories_configuration_enabled", true)             // controls the agent configurations
	config.BindEnvAndSetDefault("inventories_checks_configuration_enabled", true)      // controls the checks configurations
	config.BindEnvAndSetDefault("inventories_collect_cloud_provider_account_id", true) // collect collection of `cloud_provider_account_id`
	// when updating the default here also update pkg/metadata/inventories/README.md
	config.BindEnvAndSetDefault("inventories_max_interval", 0) // 0 == default interval from inventories
	config.BindEnvAndSetDefault("inventories_min_interval", 0) // 0 == default interval from inventories
	// Seconds to wait to sent metadata payload to the backend after startup
	config.BindEnvAndSetDefault("inventories_first_run_delay", 60)

	// Datadog security agent (common)
	config.BindEnvAndSetDefault("security_agent.cmd_port", DefaultSecurityAgentCmdPort)
	config.BindEnvAndSetDefault("security_agent.expvar_port", 5011)
	config.BindEnvAndSetDefault("security_agent.log_file", DefaultSecurityAgentLogFile)
	config.BindEnvAndSetDefault("security_agent.remote_tagger", true)
	config.BindEnvAndSetDefault("security_agent.remote_workloadmeta", true)

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

	// Datadog security agent (compliance)
	config.BindEnvAndSetDefault("compliance_config.enabled", false)
	config.BindEnvAndSetDefault("compliance_config.xccdf.enabled", false) // deprecated, use host_benchmarks instead
	config.BindEnvAndSetDefault("compliance_config.host_benchmarks.enabled", true)
	config.BindEnvAndSetDefault("compliance_config.database_benchmarks.enabled", false)
	config.BindEnvAndSetDefault("compliance_config.check_interval", 20*time.Minute)
	config.BindEnvAndSetDefault("compliance_config.check_max_events_per_run", 100)
	config.BindEnvAndSetDefault("compliance_config.dir", "/etc/datadog-agent/compliance.d")
	config.BindEnv("compliance_config.run_commands_as")
	bindEnvAndSetLogsConfigKeys(config, "compliance_config.endpoints.")
	config.BindEnvAndSetDefault("compliance_config.metrics.enabled", false)
	config.BindEnvAndSetDefault("compliance_config.opa.metrics.enabled", false)

	// Datadog security agent (runtime)
	config.BindEnvAndSetDefault("runtime_security_config.enabled", false)
	if runtime.GOOS == "windows" {
		config.BindEnvAndSetDefault("runtime_security_config.socket", "localhost:3334")
	} else {
		config.BindEnvAndSetDefault("runtime_security_config.socket", filepath.Join(InstallPath, "run/runtime-security.sock"))
	}
	config.BindEnvAndSetDefault("runtime_security_config.log_profiled_workloads", false)
	config.BindEnvAndSetDefault("runtime_security_config.use_secruntime_track", true)
	bindEnvAndSetLogsConfigKeys(config, "runtime_security_config.endpoints.")
	bindEnvAndSetLogsConfigKeys(config, "runtime_security_config.activity_dump.remote_storage.endpoints.")

	// trace-agent's evp_proxy
	config.BindEnv("evp_proxy_config.enabled")
	config.BindEnv("evp_proxy_config.dd_url")
	config.BindEnv("evp_proxy_config.api_key")
	config.BindEnv("evp_proxy_config.additional_endpoints")
	config.BindEnv("evp_proxy_config.max_payload_size")
	config.BindEnv("evp_proxy_config.receiver_timeout")

	// command line options
	config.SetKnown("cmd.check.fullsketches")

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

	// Datadog Agent Manager System Tray
	config.BindEnvAndSetDefault("system_tray.log_file", "")

	// Language Detection
	config.BindEnvAndSetDefault("language_detection.enabled", false)
	config.BindEnvAndSetDefault("language_detection.reporting.enabled", true)
	// buffer period represents how frequently newly detected languages buffer is flushed by reporting its content to the language detection handler in the cluster agent
	config.BindEnvAndSetDefault("language_detection.reporting.buffer_period", "10s")
	// TTL refresh period represents how frequently actively detected languages are refreshed by reporting them again to the language detection handler in the cluster agent
	config.BindEnvAndSetDefault("language_detection.reporting.refresh_period", "20m")

	setupProcesses(config)

	// Installer configuration
	config.BindEnvAndSetDefault("remote_updates", false)
	config.BindEnvAndSetDefault("remote_policies", false)
	config.BindEnvAndSetDefault("installer.registry.url", "")
	config.BindEnvAndSetDefault("installer.registry.auth", "")
	config.BindEnvAndSetDefault("installer.registry.username", "")
	config.BindEnvAndSetDefault("installer.registry.password", "")
	config.BindEnv("fleet_policies_dir")
	config.SetDefault("fleet_layers", []string{})

	// Data Jobs Monitoring config
	config.BindEnvAndSetDefault("djm_config.enabled", false)

	// Reverse DNS Enrichment
	config.SetKnown("reverse_dns_enrichment.workers")
	config.SetKnown("reverse_dns_enrichment.chan_size")
	config.BindEnvAndSetDefault("reverse_dns_enrichment.rate_limiter.enabled", true)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.cache.enabled", true)
	config.BindEnvAndSetDefault("reverse_dns_enrichment.cache.entry_ttl", time.Duration(0))
	config.BindEnvAndSetDefault("reverse_dns_enrichment.cache.clean_interval", time.Duration(0))
	config.BindEnvAndSetDefault("reverse_dns_enrichment.cache.persist_interval", time.Duration(0))
	config.BindEnvAndSetDefault("reverse_dns_enrichment.cache.max_retries", -1)
	config.SetKnown("reverse_dns_enrichment.cache.max_size")
	config.SetKnown("reverse_dns_enrichment.rate_limiter.limit_per_sec")
	config.SetKnown("reverse_dns_enrichment.rate_limiter.limit_throttled_per_sec")
	config.SetKnown("reverse_dns_enrichment.rate_limiter.throttle_error_threshold")
	config.SetKnown("reverse_dns_enrichment.rate_limiter.recovery_intervals")
	config.BindEnvAndSetDefault("reverse_dns_enrichment.rate_limiter.recovery_interval", time.Duration(0))
}

func agent(config pkgconfigmodel.Setup) {
	config.BindEnv("api_key")

	// Agent
	// Don't set a default on 'site' to allow detecting with viper whether it's set in config
	config.BindEnv("site")
	config.BindEnv("dd_url", "DD_DD_URL", "DD_URL")
	config.BindEnvAndSetDefault("app_key", "")
	config.BindEnvAndSetDefault("cloud_provider_metadata", []string{"aws", "gcp", "azure", "alibaba", "oracle", "ibm"})
	config.SetDefault("proxy", nil)
	config.BindEnvAndSetDefault("skip_ssl_validation", false)
	config.BindEnvAndSetDefault("sslkeylogfile", "")
	config.BindEnv("tls_handshake_timeout")
	config.BindEnv("http_dial_fallback_delay")
	config.BindEnvAndSetDefault("hostname", "")
	config.BindEnvAndSetDefault("hostname_file", "")
	config.BindEnvAndSetDefault("tags", []string{})
	config.BindEnvAndSetDefault("extra_tags", []string{})
	// If enabled, all origin detection mechanisms will be unified to use the same logic.
	// Will override all other origin detection settings in favor of the unified one.
	config.BindEnvAndSetDefault("origin_detection_unified", false)
	config.BindEnv("env")
	config.BindEnvAndSetDefault("tag_value_split_separator", map[string]string{})
	config.BindEnvAndSetDefault("conf_path", ".")
	config.BindEnvAndSetDefault("confd_path", defaultConfdPath)
	config.BindEnvAndSetDefault("additional_checksd", defaultAdditionalChecksPath)
	config.BindEnvAndSetDefault("jmx_log_file", "")
	// If enabling log_payloads, ensure the log level is set to at least DEBUG to be able to see the logs
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
	config.BindEnv("ipc_address") // deprecated: use `cmd_host` instead
	config.BindEnvAndSetDefault("cmd_host", "localhost")
	config.BindEnvAndSetDefault("cmd_port", 5001)
	config.BindEnvAndSetDefault("agent_ipc.host", "localhost")
	config.BindEnvAndSetDefault("agent_ipc.port", 0)
	config.BindEnvAndSetDefault("agent_ipc.config_refresh_interval", 0)
	config.BindEnvAndSetDefault("default_integration_http_timeout", 9)
	config.BindEnvAndSetDefault("integration_tracing", false)
	config.BindEnvAndSetDefault("integration_tracing_exhaustive", false)
	config.BindEnvAndSetDefault("integration_profiling", false)
	config.BindEnvAndSetDefault("integration_check_status_enabled", false)
	config.BindEnvAndSetDefault("enable_metadata_collection", true)
	config.BindEnvAndSetDefault("enable_gohai", true)
	config.BindEnvAndSetDefault("enable_signing_metadata_collection", true)
	config.BindEnvAndSetDefault("metadata_provider_stop_timeout", 30*time.Second)
	config.BindEnvAndSetDefault("check_runners", int64(4))
	config.BindEnvAndSetDefault("check_cancel_timeout", 500*time.Millisecond)
	config.BindEnvAndSetDefault("auth_token_file_path", "")
	config.BindEnv("bind_host")
	config.BindEnvAndSetDefault("health_port", int64(0))
	config.BindEnvAndSetDefault("disable_py3_validation", false)
	config.BindEnvAndSetDefault("python_version", DefaultPython)
	config.BindEnvAndSetDefault("win_skip_com_init", false)
	config.BindEnvAndSetDefault("allow_arbitrary_tags", false)
	config.BindEnvAndSetDefault("use_proxy_for_cloud_metadata", false)
	config.BindEnvAndSetDefault("remote_tagger_timeout_seconds", 30)

	// Configuration for TLS for outgoing connections
	config.BindEnvAndSetDefault("min_tls_version", "tlsv1.2")

	// Use to output logs in JSON format
	config.BindEnvAndSetDefault("log_format_json", false)

	// Agent GUI access host
	// 		'http://localhost' is preferred over 'http://127.0.0.1' due to Internet Explorer behavior.
	// 		Internet Explorer High Security Level does not support setting cookies via HTTP Header response.
	// 		By default, 'http://localhost' is categorized as an "intranet" website, which is considered safer and allowed to use cookies. This is not the case for 'http://127.0.0.1'.
	config.BindEnvAndSetDefault("GUI_host", "localhost")
	// Agent GUI access port
	config.BindEnvAndSetDefault("GUI_port", defaultGuiPort)
	config.BindEnvAndSetDefault("GUI_session_expiration", 0)

	config.SetKnown("proxy.http")
	config.SetKnown("proxy.https")
	config.SetKnown("proxy.no_proxy")
}

func fips(config pkgconfigmodel.Setup) {
	// Fips
	config.BindEnvAndSetDefault("fips.enabled", false)
	config.BindEnvAndSetDefault("fips.port_range_start", 9803)
	config.BindEnvAndSetDefault("fips.local_address", "localhost")
	config.BindEnvAndSetDefault("fips.https", true)
	config.BindEnvAndSetDefault("fips.tls_verify", true)
}

func remoteconfig(config pkgconfigmodel.Setup) {
	// Remote config
	config.BindEnvAndSetDefault("remote_configuration.enabled", true)
	config.BindEnvAndSetDefault("remote_configuration.key", "")
	config.BindEnv("remote_configuration.api_key")
	config.BindEnv("remote_configuration.rc_dd_url")
	config.BindEnvAndSetDefault("remote_configuration.no_tls", false)
	config.BindEnvAndSetDefault("remote_configuration.no_tls_validation", false)
	config.BindEnvAndSetDefault("remote_configuration.config_root", "")
	config.BindEnvAndSetDefault("remote_configuration.director_root", "")
	config.BindEnv("remote_configuration.refresh_interval")
	config.BindEnvAndSetDefault("remote_configuration.max_backoff_interval", 5*time.Minute)
	config.BindEnvAndSetDefault("remote_configuration.clients.ttl_seconds", 30*time.Second)
	config.BindEnvAndSetDefault("remote_configuration.clients.cache_bypass_limit", 5)
	// Remote config products
	config.BindEnvAndSetDefault("remote_configuration.apm_sampling.enabled", true)
	config.BindEnvAndSetDefault("remote_configuration.agent_integrations.enabled", false)
	config.BindEnvAndSetDefault("remote_configuration.agent_integrations.allow_list", defaultAllowedRCIntegrations)
	config.BindEnvAndSetDefault("remote_configuration.agent_integrations.block_list", []string{})
	config.BindEnvAndSetDefault("remote_configuration.agent_integrations.allow_log_config_scheduling", false)
}

func autoconfig(config pkgconfigmodel.Setup) {
	// Autoconfig
	// Defaut Timeout in second when talking to storage for configuration (etcd, zookeeper, ...)
	config.BindEnvAndSetDefault("autoconf_template_url_timeout", 5)
	// Where to look for check templates if no custom path is defined
	config.BindEnvAndSetDefault("autoconf_template_dir", "/datadog/check_configs")
	config.BindEnvAndSetDefault("autoconf_config_files_poll", false)
	config.BindEnvAndSetDefault("autoconf_config_files_poll_interval", 60)
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
}

func containerSyspath(config pkgconfigmodel.Setup) {
	if pkgconfigenv.IsContainerized() {
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
	config.BindEnv("container_pid_mapper")
	config.BindEnvAndSetDefault("ignore_host_etc", false)
	config.BindEnvAndSetDefault("use_improved_cgroup_parser", false)
	config.BindEnvAndSetDefault("proc_root", "/proc")
}

func debugging(config pkgconfigmodel.Setup) {
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
	config.BindEnv("no_proxy_nonexact_match")
}

func telemetry(config pkgconfigmodel.Setup) {
	// Telemetry
	// Enable telemetry metrics on the internals of the Agent.
	// This create a lot of billable custom metrics.
	config.BindEnvAndSetDefault("telemetry.enabled", false)
	config.BindEnvAndSetDefault("telemetry.dogstatsd_origin", false)
	config.BindEnvAndSetDefault("telemetry.python_memory", true)
	config.BindEnv("telemetry.checks")
	// We're using []string as a default instead of []float64 because viper can only parse list of string from the environment
	//
	// The histogram buckets use to track the time in nanoseconds DogStatsD listeners are not reading/waiting new data
	config.BindEnvAndSetDefault("telemetry.dogstatsd.listeners_latency_buckets", []string{})
	// The histogram buckets use to track the time in nanoseconds it takes for the DogStatsD server to push data to the aggregator
	config.BindEnvAndSetDefault("telemetry.dogstatsd.aggregator_channel_latency_buckets", []string{})
	// The histogram buckets use to track the time in nanoseconds it takes for a DogStatsD listeners to push data to the server
	config.BindEnvAndSetDefault("telemetry.dogstatsd.listeners_channel_latency_buckets", []string{})

	// Agent Telemetry
	config.BindEnvAndSetDefault("agent_telemetry.enabled", true)
	config.SetKnown("agent_telemetry.additional_endpoints.*")
	bindEnvAndSetLogsConfigKeys(config, "agent_telemetry.")

}

func serializer(config pkgconfigmodel.Setup) {
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
	config.BindEnvAndSetDefault("serializer_compressor_kind", DefaultCompressorKind)
	config.BindEnvAndSetDefault("serializer_zstd_compressor_level", DefaultZstdCompressionLevel)

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
	config.BindEnvAndSetDefault("basic_telemetry_add_container_tags", false) // configure adding the agent container tags to the basic agent telemetry metrics (e.g. `datadog.agent.running`)
	config.BindEnvAndSetDefault("aggregator_flush_metrics_and_serialize_in_parallel_chan_size", 200)
	config.BindEnvAndSetDefault("aggregator_flush_metrics_and_serialize_in_parallel_buffer_size", 4000)
}

func serverless(config pkgconfigmodel.Setup) {
	// Serverless Agent
	config.SetDefault("serverless.enabled", false)
	config.BindEnvAndSetDefault("serverless.logs_enabled", true)
	config.BindEnvAndSetDefault("enhanced_metrics", true)
	config.BindEnvAndSetDefault("capture_lambda_payload", false)
	config.BindEnvAndSetDefault("capture_lambda_payload_max_depth", 10)
	config.BindEnvAndSetDefault("serverless.trace_enabled", true, "DD_TRACE_ENABLED")
	config.BindEnvAndSetDefault("serverless.trace_managed_services", true, "DD_TRACE_MANAGED_SERVICES")
	config.BindEnvAndSetDefault("serverless.service_mapping", nil, "DD_SERVICE_MAPPING")
}

func forwarder(config pkgconfigmodel.Setup) {
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
}

func dogstatsd(config pkgconfigmodel.Setup) {
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
	config.BindEnvAndSetDefault("dogstatsd_socket", defaultStatsdSocket) // Only enabled on unix systems
	config.BindEnvAndSetDefault("dogstatsd_stream_socket", "")           // Experimental || Notice: empty means feature disabled
	config.BindEnvAndSetDefault("dogstatsd_pipeline_autoadjust", false)
	config.BindEnvAndSetDefault("dogstatsd_pipeline_autoadjust_strategy", "max_throughput")
	config.BindEnvAndSetDefault("dogstatsd_pipeline_count", 1)
	config.BindEnvAndSetDefault("dogstatsd_stats_port", 5000)
	config.BindEnvAndSetDefault("dogstatsd_stats_enable", false)
	config.BindEnvAndSetDefault("dogstatsd_stats_buffer", 10)
	config.BindEnvAndSetDefault("dogstatsd_telemetry_enabled_listener_id", false)
	// Control how dogstatsd-stats logs can be generated
	config.BindEnvAndSetDefault("dogstatsd_log_file", "")
	config.BindEnvAndSetDefault("dogstatsd_logging_enabled", true)
	config.BindEnvAndSetDefault("dogstatsd_log_file_max_rolls", 3)
	config.BindEnvAndSetDefault("dogstatsd_log_file_max_size", "10Mb")
	// Control for how long counter would be sampled to 0 if not received
	config.BindEnvAndSetDefault("dogstatsd_expiry_seconds", 300)
	// Control how long we keep dogstatsd contexts in memory.
	config.BindEnvAndSetDefault("dogstatsd_context_expiry_seconds", 20)
	config.BindEnvAndSetDefault("dogstatsd_origin_detection", false) // Only supported for socket traffic
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
	// Force the amount of dogstatsd workers (mainly used for benchmarks or some very specific use-case)
	config.BindEnvAndSetDefault("dogstatsd_workers_count", 0)

	// To enable the following feature, GODEBUG must contain `madvdontneed=1`
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.enabled", false)
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.low_soft_limit", 0.7)
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.high_soft_limit", 0.8)
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.go_gc", 1) // 0 means don't call SetGCPercent
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.memory_ballast", int64(1024*1024*1024*8))
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.rate_check.min", 0.01)
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.rate_check.max", 1)
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.rate_check.factor", 2)
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.soft_limit_freeos_check.min", 0.01)
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.soft_limit_freeos_check.max", 0.1)
	config.BindEnvAndSetDefault("dogstatsd_mem_based_rate_limiter.soft_limit_freeos_check.factor", 1.5)

	config.BindEnv("dogstatsd_mapper_profiles")
	config.ParseEnvAsSlice("dogstatsd_mapper_profiles", func(in string) []interface{} {
		var mappings []interface{}
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
	config.BindEnvAndSetDefault("statsd_metric_blocklist_match_prefix", false)

	config.BindEnvAndSetDefault("histogram_copy_to_distribution", false)
	config.BindEnvAndSetDefault("histogram_copy_to_distribution_prefix", "")
	config.BindEnvAndSetDefault("histogram_aggregates", []string{"max", "median", "avg", "count"})
	config.BindEnvAndSetDefault("histogram_percentiles", []string{"0.95"})
}

func logsagent(config pkgconfigmodel.Setup) {
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
	// maximum log message size in bytes
	config.BindEnvAndSetDefault("logs_config.max_message_size_bytes", DefaultMaxMessageSizeBytes)

	// increase the number of files that can be tailed in parallel:
	if runtime.GOOS == "darwin" {
		// The default limit on darwin is 256.
		// This is configurable per process on darwin with `ulimit -n` or a launchDaemon config.
		config.BindEnvAndSetDefault("logs_config.open_files_limit", 200)
	} else {
		// There is no effective limit for windows due to use of CreateFile win32 API
		// The OS default for most linux distributions is 1024
		config.BindEnvAndSetDefault("logs_config.open_files_limit", 500)
	}
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
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.aurora.enabled", false)
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.aurora.discovery_interval", 300)
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.aurora.region", "")
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.aurora.query_timeout", 10)
	config.BindEnvAndSetDefault("database_monitoring.autodiscovery.aurora.tags", []string{"datadoghq.com/scrape:true"})

	config.BindEnvAndSetDefault("logs_config.dd_port", 10516)
	config.BindEnvAndSetDefault("logs_config.dev_mode_use_proto", true)
	config.BindEnvAndSetDefault("logs_config.dd_url_443", "agent-443-intake.logs.datadoghq.com")
	config.BindEnvAndSetDefault("logs_config.stop_grace_period", 30)
	// maximum time that the unix tailer will hold a log file open after it has been rotated
	config.BindEnvAndSetDefault("logs_config.close_timeout", 60)
	// maximum time that the windows tailer will hold a log file open, while waiting for
	// the downstream logs pipeline to be ready to accept more data
	config.BindEnvAndSetDefault("logs_config.windows_open_file_timeout", 5)

	config.BindEnvAndSetDefault("logs_config.auto_multi_line_detection", false)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_extra_patterns", []string{})
	// The following auto_multi_line settings are experimental and may change
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_default_sample_size", 500)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_default_match_timeout", 30) // Seconds
	config.BindEnvAndSetDefault("logs_config.auto_multi_line_default_match_threshold", 0.48)

	// Experimental auto multiline detection settings (these are subject to change until the feature is no longer experimental)
	config.BindEnvAndSetDefault("logs_config.experimental_auto_multi_line_detection", false)
	config.SetKnown("logs_config.auto_multi_line_detection_custom_samples")
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.enable_json_detection", true)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.enable_datetime_detection", true)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.timestamp_detector_match_threshold", 0.5)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.tokenizer_max_input_bytes", 60)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.pattern_table_max_size", 20)
	config.BindEnvAndSetDefault("logs_config.auto_multi_line.pattern_table_match_threshold", 0.75)
	// Add a tag to logs that are multiline aggregated
	config.BindEnvAndSetDefault("logs_config.tag_multi_line_logs", false)
	// Add a tag to logs that are truncated by the agent
	config.BindEnvAndSetDefault("logs_config.tag_truncated_logs", false)
	config.BindEnvAndSetDefault("logs_config.chan_size", 100)

	// If true, the agent looks for container logs in the location used by podman, rather
	// than docker.  This is a temporary configuration parameter to support podman logs until
	// a more substantial refactor of autodiscovery is made to determine this automatically.
	config.BindEnvAndSetDefault("logs_config.use_podman_logs", false)

	// If true, then a source_host tag (IP Address) will be added to TCP/UDP logs.
	config.BindEnvAndSetDefault("logs_config.use_sourcehost_tag", true)

	// If set, the agent will look in this path for docker container log files.  Use this option if
	// docker's `data-root` has been set to a custom path and you wish to ingest docker logs from files. In
	// order to check your docker data-root directory, run the command `docker info -f '{{.DockerRootDir}}'`
	// See more documentation here:
	// https://docs.docker.com/engine/reference/commandline/dockerd/.
	config.BindEnvAndSetDefault("logs_config.docker_path_override", "")

	config.BindEnvAndSetDefault("logs_config.auditor_ttl", DefaultAuditorTTL) // in hours
	// Timeout in milliseonds used when performing agreggation operations,
	// including multi-line log processing rules and chunked line reaggregation.
	// It may be useful to increase it when logs writing is slowed down, that
	// could happen while serializing large objects on log lines.
	config.BindEnvAndSetDefault("logs_config.aggregation_timeout", 1000)
	// Time in seconds
	config.BindEnvAndSetDefault("logs_config.file_scan_period", 10.0)

	// Controls how wildcard file log source are prioritized when there are more files
	// that match wildcard log configurations than the `logs_config.open_files_limit`
	//
	// Choices are 'by_name' and 'by_modification_time'. See config_template.yaml for full details.
	//
	// WARNING: 'by_modification_time' is less performant than 'by_name' and will trigger
	// more disk I/O at the wildcard log paths
	config.BindEnvAndSetDefault("logs_config.file_wildcard_selection_mode", "by_name")

	// Max size in MB an integration logs file can use
	config.BindEnvAndSetDefault("logs_config.integrations_logs_files_max_size", 10)
	// Max disk usage in MB all integrations logs files are allowed to use in total
	config.BindEnvAndSetDefault("logs_config.integrations_logs_total_usage", 100)
	// Do not store logs on disk when the disk usage exceeds 80% of the disk capacity.
	config.BindEnvAndSetDefault("logs_config.integrations_logs_disk_ratio", 0.80)

	// SDS logs blocking mechanism
	config.BindEnvAndSetDefault("logs_config.sds.wait_for_configuration", "")
	config.BindEnvAndSetDefault("logs_config.sds.buffer_max_size", 0)
}

func vector(config pkgconfigmodel.Setup) {
	// Vector integration
	bindVectorOptions(config, Metrics)
	bindVectorOptions(config, Logs)
}

func cloudfoundry(config pkgconfigmodel.Setup) {
	// Cloud Foundry
	config.BindEnvAndSetDefault("cloud_foundry", false)
	config.BindEnvAndSetDefault("bosh_id", "")
	config.BindEnvAndSetDefault("cf_os_hostname_aliasing", false)
	config.BindEnvAndSetDefault("cloud_foundry_buildpack", false)
}

func containerd(config pkgconfigmodel.Setup) {
	// Containerd
	config.BindEnvAndSetDefault("containerd_namespace", []string{})
	config.BindEnvAndSetDefault("containerd_namespaces", []string{}) // alias for containerd_namespace
	config.BindEnvAndSetDefault("containerd_exclude_namespaces", []string{"moby"})
	config.BindEnvAndSetDefault("container_env_as_tags", map[string]string{})
	config.BindEnvAndSetDefault("container_labels_as_tags", map[string]string{})
}

func cri(config pkgconfigmodel.Setup) {
	// CRI
	config.BindEnvAndSetDefault("cri_socket_path", "")              // empty is disabled
	config.BindEnvAndSetDefault("cri_connection_timeout", int64(1)) // in seconds
	config.BindEnvAndSetDefault("cri_query_timeout", int64(5))      // in seconds
}

func kubernetes(config pkgconfigmodel.Setup) {
	// Kubernetes
	config.BindEnvAndSetDefault("kubernetes_kubelet_host", "")
	config.BindEnvAndSetDefault("kubernetes_kubelet_nodename", "")
	config.BindEnvAndSetDefault("eks_fargate", false)
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

	config.BindEnvAndSetDefault("kubernetes_pod_expiration_duration", 15*60) // in seconds, default 15 minutes
	config.BindEnvAndSetDefault("kubelet_wait_on_missing_container", 0)
	config.BindEnvAndSetDefault("kubelet_cache_pods_duration", 5)       // Polling frequency in seconds of the agent to the kubelet "/pods" endpoint
	config.BindEnvAndSetDefault("kubelet_listener_polling_interval", 5) // Polling frequency in seconds of the pod watcher to detect new pods/containers (affected by kubelet_cache_pods_duration setting)
	config.BindEnvAndSetDefault("kubernetes_collect_metadata_tags", true)
	config.BindEnvAndSetDefault("kubernetes_metadata_tag_update_freq", 60) // Polling frequency of the Agent to the DCA in seconds (gets the local cache if the DCA is disabled)
	config.BindEnvAndSetDefault("kubernetes_apiserver_client_timeout", 10)
	config.BindEnvAndSetDefault("kubernetes_apiserver_informer_client_timeout", 0)
	config.BindEnvAndSetDefault("kubernetes_map_services_on_ip", false) // temporary opt-out of the new mapping logic
	config.BindEnvAndSetDefault("kubernetes_apiserver_use_protobuf", false)
	config.BindEnvAndSetDefault("kubernetes_ad_tags_disabled", []string{})
}

func podman(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("podman_db_path", "")
}

// LoadProxyFromEnv overrides the proxy settings with environment variables
func LoadProxyFromEnv(config pkgconfigmodel.Config) {
	// Viper doesn't handle mixing nested variables from files and set
	// manually.  If we manually set one of the sub value for "proxy" all
	// other values from the conf file will be shadowed when using
	// 'config.Get("proxy")'. For that reason we first get the value from
	// the conf files, overwrite them with the env variables and reset
	// everything.

	// When FIPS proxy is enabled we ignore proxy setting to force data to the local proxy
	if config.GetBool("fips.enabled") {
		log.Infof("'fips.enabled' has been set to true. Ignoring proxy setting.")
		return
	}

	log.Info("Loading proxy settings")

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
	p := &pkgconfigmodel.Proxy{}
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

	if !config.GetBool("use_proxy_for_cloud_metadata") {
		log.Debugf("'use_proxy_for_cloud_metadata' is enabled: adding cloud provider URL to the no_proxy list")
		isSet = true
		p.NoProxy = append(p.NoProxy,
			"169.254.169.254", // Azure, EC2, GCE
			"100.100.100.200", // Alibaba
		)
	}

	// We have to set each value individually so both config.Get("proxy")
	// and config.Get("proxy.http") work
	if isSet {
		config.Set("proxy.http", p.HTTP, pkgconfigmodel.SourceEnvVar)
		config.Set("proxy.https", p.HTTPS, pkgconfigmodel.SourceEnvVar)

		// If this is set to an empty []string, viper will have a type conflict when merging
		// this config during secrets resolution. It unmarshals empty yaml lists to type
		// []interface{}, which will then conflict with type []string and fail to merge.
		noProxy := make([]interface{}, len(p.NoProxy))
		for idx := range p.NoProxy {
			noProxy[idx] = p.NoProxy[idx]
		}
		config.Set("proxy.no_proxy", noProxy, pkgconfigmodel.SourceEnvVar)
	}
}

// LoadWithoutSecret reads configs files, initializes the config module without decrypting any secrets
func LoadWithoutSecret(config pkgconfigmodel.Config, additionalEnvVars []string) (*pkgconfigmodel.Warnings, error) {
	return LoadDatadogCustom(config, "datadog.yaml", optional.NewNoneOption[secrets.Component](), additionalEnvVars)
}

// LoadWithSecret reads config files and initializes config with decrypted secrets
func LoadWithSecret(config pkgconfigmodel.Config, secretResolver secrets.Component, additionalEnvVars []string) (*pkgconfigmodel.Warnings, error) {
	return LoadDatadogCustom(config, "datadog.yaml", optional.NewOption[secrets.Component](secretResolver), additionalEnvVars)
}

// Merge will merge additional configuration into an existing configuration
func Merge(configPaths []string, config pkgconfigmodel.Config) error {
	for _, configPath := range configPaths {
		if f, err := os.Open(configPath); err == nil {
			err = config.MergeConfig(f)
			_ = f.Close()
			if err != nil {
				return fmt.Errorf("error merging %s config file: %w", configPath, err)
			}
		} else {
			log.Infof("no config exists at %s, ignoring...", configPath)
		}
	}

	return nil
}

func findUnknownKeys(config pkgconfigmodel.Config) []string {
	var unknownKeys []string
	knownKeys := config.GetKnownKeysLowercased()
	loadedKeys := config.AllKeysLowercased()
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

func findUnexpectedUnicode(config pkgconfigmodel.Config) []string {
	messages := make([]string, 0)
	checkAndRecordString := func(str string, prefix string) {
		if res := FindUnexpectedUnicode(str); len(res) != 0 {
			for _, detected := range res {
				msg := fmt.Sprintf("%s - Unexpected unicode %s codepoint '%U' detected at byte position %v", prefix, detected.reason, detected.codepoint, detected.position)
				messages = append(messages, msg)
			}
		}
	}

	var visitElement func(string, interface{})
	visitElement = func(key string, element interface{}) {
		switch elementValue := element.(type) {
		case string:
			checkAndRecordString(elementValue, fmt.Sprintf("For key '%s', configuration value string '%s'", key, elementValue))
		case []string:
			for _, s := range elementValue {
				checkAndRecordString(s, fmt.Sprintf("For key '%s', configuration value string '%s'", key, s))
			}
		case []interface{}:
			for _, listItem := range elementValue {
				visitElement(key, listItem)
			}
		}
	}

	allKeys := config.AllKeysLowercased()
	for _, key := range allKeys {
		checkAndRecordString(key, fmt.Sprintf("Configuration key string '%s'", key))
		if unknownValue := config.Get(key); unknownValue != nil {
			visitElement(key, unknownValue)
		}
	}

	return messages
}

func findUnknownEnvVars(config pkgconfigmodel.Config, environ []string, additionalKnownEnvVars []string) []string {
	var unknownVars []string

	knownVars := map[string]struct{}{
		// these variables are used by the agent, but not via the Config struct,
		// so must be listed separately.
		"DD_INSIDE_CI":      {},
		"DD_PROXY_HTTP":     {},
		"DD_PROXY_HTTPS":    {},
		"DD_PROXY_NO_PROXY": {},
		// these variables are used by serverless, but not via the Config struct
		"DD_AAS_DOTNET_EXTENSION_VERSION":          {},
		"DD_AAS_EXTENSION_VERSION":                 {},
		"DD_AAS_JAVA_EXTENSION_VERSION":            {},
		"DD_AGENT_PIPE_NAME":                       {},
		"DD_API_KEY_SECRET_ARN":                    {},
		"DD_APM_FLUSH_DEADLINE_MILLISECONDS":       {},
		"DD_APPSEC_ENABLED":                        {},
		"DD_AZURE_APP_SERVICES":                    {},
		"DD_DOGSTATSD_ARGS":                        {},
		"DD_DOGSTATSD_PATH":                        {},
		"DD_DOGSTATSD_WINDOWS_PIPE_NAME":           {},
		"DD_DOTNET_TRACER_HOME":                    {},
		"DD_EXTENSION_PATH":                        {},
		"DD_FLUSH_TO_LOG":                          {},
		"DD_KMS_API_KEY":                           {},
		"DD_INTEGRATIONS":                          {},
		"DD_INTERNAL_NATIVE_LOADER_PATH":           {},
		"DD_INTERNAL_PROFILING_NATIVE_ENGINE_PATH": {},
		"DD_LAMBDA_HANDLER":                        {},
		"DD_LOGS_INJECTION":                        {},
		"DD_MERGE_XRAY_TRACES":                     {},
		"DD_PROFILER_EXCLUDE_PROCESSES":            {},
		"DD_PROFILING_LOG_DIR":                     {},
		"DD_RUNTIME_METRICS_ENABLED":               {},
		"DD_SERVERLESS_APPSEC_ENABLED":             {},
		"DD_SERVERLESS_FLUSH_STRATEGY":             {},
		"DD_SERVICE":                               {},
		"DD_TRACE_AGENT_ARGS":                      {},
		"DD_TRACE_AGENT_PATH":                      {},
		"DD_TRACE_AGENT_URL":                       {},
		"DD_TRACE_LOG_DIRECTORY":                   {},
		"DD_TRACE_LOG_PATH":                        {},
		"DD_TRACE_METRICS_ENABLED":                 {},
		"DD_TRACE_PIPE_NAME":                       {},
		"DD_TRACE_TRANSPORT":                       {},
		"DD_VERSION":                               {},
		// this variable is used by the Kubernetes leader election mechanism
		"DD_POD_NAME": {},
		// this variable is used by tracers
		"DD_INSTRUMENTATION_TELEMETRY_ENABLED": {},
		// these variables are used by source code integration
		"DD_GIT_COMMIT_SHA":     {},
		"DD_GIT_REPOSITORY_URL": {},
	}
	for _, key := range config.GetEnvVars() {
		knownVars[key] = struct{}{}
	}
	for _, key := range additionalKnownEnvVars {
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

func useHostEtc(config pkgconfigmodel.Config) {
	if pkgconfigenv.IsContainerized() && pathExists("/host/etc") {
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

func checkConflictingOptions(config pkgconfigmodel.Config) error {
	// Verify that either use_podman_logs OR docker_path_override are set since they conflict
	if config.GetBool("logs_config.use_podman_logs") && len(config.GetString("logs_config.docker_path_override")) > 0 {
		log.Warnf("'use_podman_logs' is set to true and 'docker_path_override' is set, please use one or the other")
		return errors.New("'use_podman_logs' is set to true and 'docker_path_override' is set, please use one or the other")
	}

	return nil
}

// LoadDatadogCustom loads the datadog config in the given config
func LoadDatadogCustom(config pkgconfigmodel.Config, origin string, secretResolver optional.Option[secrets.Component], additionalKnownEnvVars []string) (*pkgconfigmodel.Warnings, error) {
	// Feature detection running in a defer func as it always  need to run (whether config load has been successful or not)
	// Because some Agents (e.g. trace-agent) will run even if config file does not exist
	defer func() {
		// Environment feature detection needs to run before applying override funcs
		// as it may provide such overrides
		pkgconfigenv.DetectFeatures(config)
		pkgconfigmodel.ApplyOverrideFuncs(config)
	}()

	warnings := &pkgconfigmodel.Warnings{}
	err := LoadCustom(config, additionalKnownEnvVars)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return warnings, log.Warnf("Error loading config: %v (check config file permissions for dd-agent user)", err)
		}
		return warnings, err
	}

	// We resolve proxy setting before secrets. This allows setting secrets through DD_PROXY_* env variables
	LoadProxyFromEnv(config)

	if resolver, ok := secretResolver.Get(); ok {
		if err := ResolveSecrets(config, resolver, origin); err != nil {
			return warnings, err
		}
	}

	// Verify 'DD_URL' and 'DD_DD_URL' conflicts
	if EnvVarAreSetAndNotEqual("DD_DD_URL", "DD_URL") {
		log.Warnf("'DD_URL' and 'DD_DD_URL' variables are both set in environment. Using 'DD_DD_URL' value")
	}

	useHostEtc(config)

	err = checkConflictingOptions(config)
	if err != nil {
		return warnings, err
	}

	// If this variable is set to true, we'll use DefaultPython for the Python version,
	// ignoring the python_version configuration value.
	if ForceDefaultPython == "true" && config.IsKnown("python_version") {
		pv := config.GetString("python_version")
		if pv != DefaultPython {
			log.Warnf("Python version has been forced to %s", DefaultPython)
		}

		pkgconfigmodel.AddOverride("python_version", DefaultPython)
	}

	sanitizeAPIKeyConfig(config, "api_key")
	sanitizeAPIKeyConfig(config, "logs_config.api_key")
	// setTracemallocEnabled *must* be called before setNumWorkers
	warnings.TraceMallocEnabledWithPy2 = setTracemallocEnabled(config)
	setNumWorkers(config)

	flareStrippedKeys := config.GetStringSlice("flare_stripped_keys")
	if len(flareStrippedKeys) > 0 {
		log.Warn("flare_stripped_keys is deprecated, please use scrubber.additional_keys instead.")
		scrubber.AddStrippedKeys(flareStrippedKeys)
	}
	scrubberAdditionalKeys := config.GetStringSlice("scrubber.additional_keys")
	if len(scrubberAdditionalKeys) > 0 {
		scrubber.AddStrippedKeys(scrubberAdditionalKeys)
	}

	return warnings, setupFipsEndpoints(config)
}

// LoadCustom reads config into the provided config object
func LoadCustom(config pkgconfigmodel.Config, additionalKnownEnvVars []string) error {
	log.Info("Starting to load the configuration")
	if err := config.ReadInConfig(); err != nil {
		if pkgconfigenv.IsServerless() {
			log.Debug("No config file detected, using environment variable based configuration only")
			// The remaining code in LoadCustom is not run to keep a low cold start time
			return nil
		}
		return err
	}

	for _, key := range findUnknownKeys(config) {
		log.Warnf("Unknown key in config file: %v", key)
	}

	for _, v := range findUnknownEnvVars(config, os.Environ(), additionalKnownEnvVars) {
		log.Warnf("Unknown environment variable: %v", v)
	}

	for _, warningMsg := range findUnexpectedUnicode(config) {
		log.Warnf("%s", warningMsg)
	}

	return nil
}

// setupFipsEndpoints overwrites the Agent endpoint for outgoing data to be sent to the local FIPS proxy. The local FIPS
// proxy will be in charge of forwarding data to the Datadog backend following FIPS standard. Starting from
// fips.port_range_start we will assign a dedicated port per product (metrics, logs, traces, ...).
func setupFipsEndpoints(config pkgconfigmodel.Config) error {
	// Each port is dedicated to a specific data type:
	//
	// port_range_start: HAProxy stats
	// port_range_start + 1:  metrics
	// port_range_start + 2:  traces
	// port_range_start + 3:  profiles
	// port_range_start + 4:  processes
	// port_range_start + 5:  logs
	// port_range_start + 6:  databases monitoring metrics, metadata and activity
	// port_range_start + 7:  databases monitoring samples
	// port_range_start + 8:  network devices metadata
	// port_range_start + 9:  network devices snmp traps
	// port_range_start + 10: instrumentation telemetry
	// port_range_start + 11: appsec events (unused)
	// port_range_start + 12: orchestrator explorer
	// port_range_start + 13: runtime security
	// port_range_start + 14: compliance
	// port_range_start + 15: network devices netflow

	if !config.GetBool("fips.enabled") {
		log.Debug("FIPS mode is disabled")
		return nil
	}

	const (
		proxyStats                 = 0
		metrics                    = 1
		traces                     = 2
		profiles                   = 3
		processes                  = 4
		logs                       = 5
		databasesMonitoringMetrics = 6
		databasesMonitoringSamples = 7
		networkDevicesMetadata     = 8
		networkDevicesSnmpTraps    = 9
		instrumentationTelemetry   = 10
		appsecEvents               = 11
		orchestratorExplorer       = 12
		runtimeSecurity            = 13
		compliance                 = 14
		networkDevicesNetflow      = 15
	)

	localAddress, err := system.IsLocalAddress(config.GetString("fips.local_address"))
	if err != nil {
		return fmt.Errorf("fips.local_address: %s", err)
	}

	portRangeStart := config.GetInt("fips.port_range_start")
	urlFor := func(port int) string { return net.JoinHostPort(localAddress, strconv.Itoa(portRangeStart+port)) }

	log.Warnf("FIPS mode is enabled! All communication to DataDog will be routed to the local FIPS proxy on '%s' starting from port %d", localAddress, portRangeStart)

	// Disabling proxy to make sure all data goes directly to the FIPS proxy
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")

	config.Set("fips.https", config.GetBool("fips.https"), pkgconfigmodel.SourceAgentRuntime)

	// HTTP for now, will soon be updated to HTTPS
	protocol := "http://"
	if config.GetBool("fips.https") {
		protocol = "https://"
		config.Set("skip_ssl_validation", !config.GetBool("fips.tls_verify"), pkgconfigmodel.SourceAgentRuntime)
	}

	// The following overwrites should be sync with the documentation for the fips.enabled config setting in the
	// config_template.yaml

	// Metrics
	config.Set("dd_url", protocol+urlFor(metrics), pkgconfigmodel.SourceAgentRuntime)

	// Logs
	setupFipsLogsConfig(config, "logs_config.", urlFor(logs))

	// APM
	config.Set("apm_config.apm_dd_url", protocol+urlFor(traces), pkgconfigmodel.SourceAgentRuntime)
	// Adding "/api/v2/profile" because it's not added to the 'apm_config.profiling_dd_url' value by the Agent
	config.Set("apm_config.profiling_dd_url", protocol+urlFor(profiles)+"/api/v2/profile", pkgconfigmodel.SourceAgentRuntime)
	config.Set("apm_config.telemetry.dd_url", protocol+urlFor(instrumentationTelemetry), pkgconfigmodel.SourceAgentRuntime)

	// Processes
	config.Set("process_config.process_dd_url", protocol+urlFor(processes), pkgconfigmodel.SourceAgentRuntime)

	// Database monitoring
	// Historically we used a different port for samples because the intake hostname defined in epforwarder.go was different
	// (even though the underlying IPs were the same as the ones for DBM metrics intake hostname). We're keeping 2 ports for backward compatibility reason.
	setupFipsLogsConfig(config, "database_monitoring.metrics.", urlFor(databasesMonitoringMetrics))
	setupFipsLogsConfig(config, "database_monitoring.activity.", urlFor(databasesMonitoringMetrics))
	setupFipsLogsConfig(config, "database_monitoring.samples.", urlFor(databasesMonitoringSamples))

	// Network devices
	// Internally, Viper uses multiple storages for the configuration values and values from datadog.yaml are stored
	// in a different place from where overrides (created with config.Set(...)) are stored.
	// Some NDM products are using UnmarshalKey() which either uses overridden data or either configuration file data but not
	// both at the same time (see https://github.com/spf13/viper/issues/1106)
	//
	// Because of that we need to put all the NDM config in the overridden data store (using Set) in order to get
	// data from the config + data created by the FIPS mode when using UnmarshalKey()

	config.Set("network_devices.snmp_traps", config.Get("network_devices.snmp_traps"), pkgconfigmodel.SourceAgentRuntime)
	setupFipsLogsConfig(config, "network_devices.metadata.", urlFor(networkDevicesMetadata))
	config.Set("network_devices.netflow", config.Get("network_devices.netflow"), pkgconfigmodel.SourceAgentRuntime)
	setupFipsLogsConfig(config, "network_devices.snmp_traps.forwarder.", urlFor(networkDevicesSnmpTraps))
	setupFipsLogsConfig(config, "network_devices.netflow.forwarder.", urlFor(networkDevicesNetflow))

	// Orchestrator Explorer
	config.Set("orchestrator_explorer.orchestrator_dd_url", protocol+urlFor(orchestratorExplorer), pkgconfigmodel.SourceAgentRuntime)

	// CWS
	setupFipsLogsConfig(config, "runtime_security_config.endpoints.", urlFor(runtimeSecurity))

	// Compliance
	setupFipsLogsConfig(config, "compliance_config.endpoints.", urlFor(compliance))

	return nil
}

func setupFipsLogsConfig(config pkgconfigmodel.Config, configPrefix string, url string) {
	config.Set(configPrefix+"use_http", true, pkgconfigmodel.SourceAgentRuntime)
	config.Set(configPrefix+"logs_no_ssl", !config.GetBool("fips.https"), pkgconfigmodel.SourceAgentRuntime)
	config.Set(configPrefix+"logs_dd_url", url, pkgconfigmodel.SourceAgentRuntime)
}

// ResolveSecrets merges all the secret values from origin into config. Secret values
// are identified by a value of the form "ENC[key]" where key is the secret key.
// See: https://github.com/DataDog/datadog-agent/blob/main/docs/agent/secrets.md
func ResolveSecrets(config pkgconfigmodel.Config, secretResolver secrets.Component, origin string) error {
	log.Info("Starting to resolve secrets")
	// We have to init the secrets package before we can use it to decrypt
	// anything.
	secretResolver.Configure(secrets.ConfigParams{
		Command:          config.GetString("secret_backend_command"),
		Arguments:        config.GetStringSlice("secret_backend_arguments"),
		Timeout:          config.GetInt("secret_backend_timeout"),
		MaxSize:          config.GetInt("secret_backend_output_max_size"),
		RefreshInterval:  config.GetInt("secret_refresh_interval"),
		GroupExecPerm:    config.GetBool("secret_backend_command_allow_group_exec_perm"),
		RemoveLinebreak:  config.GetBool("secret_backend_remove_trailing_line_break"),
		RunPath:          config.GetString("run_path"),
		AuditFileMaxSize: config.GetInt("secret_audit_file_max_size"),
	})

	if config.GetString("secret_backend_command") != "" {
		// Viper doesn't expose the final location of the file it
		// loads. Since we are searching for 'datadog.yaml' in multiple
		// locations we let viper determine the one to use before
		// updating it.
		yamlConf, err := yaml.Marshal(config.AllSettings())
		if err != nil {
			return fmt.Errorf("unable to marshal configuration to YAML to decrypt secrets: %v", err)
		}

		secretResolver.SubscribeToChanges(func(handle, settingOrigin string, settingPath []string, _, newValue any) {
			if origin != settingOrigin {
				return
			}
			if err := configAssignAtPath(config, settingPath, newValue); err != nil {
				log.Errorf("Could not assign new value of secret %s (%+q) to config: %s", handle, settingPath, err)
			}
		})
		if _, err = secretResolver.Resolve(yamlConf, origin); err != nil {
			return fmt.Errorf("unable to decrypt secret from datadog.yaml: %v", err)
		}
	}
	log.Info("Finished resolving secrets")
	return nil
}

// confgAssignAtPath assigns a value to the given setting of the config
// This works around viper issues that prevent us from assigning to fields that have a dot in the
// name (example: 'additional_endpoints.http://url.com') and also allows us to assign to individual
// elements of a slice of items (example: 'proxy.no_proxy.0' to assign index 0 of 'no_proxy')
func configAssignAtPath(config pkgconfigmodel.Config, settingPath []string, newValue any) error {
	settingName := strings.Join(settingPath, ".")
	if config.IsKnown(settingName) {
		config.Set(settingName, newValue, pkgconfigmodel.SourceAgentRuntime)
		return nil
	}

	// Trying to assign to an unknown config field can happen when trying to set a
	// value inside of a compound object (a slice or a map) which allows arbitrary key
	// values. Some settings where this happens include `additional_endpoints`, or
	// `kubernetes_node_annotations_as_tags`, etc. Since these arbitrary keys can
	// contain a '.' character, we are unable to use the standard `config.Set` method.
	// Instead, we remove trailing elements from the end of the path until we find a known
	// config field, retrieve the compound object at that point, and then use the trailing
	// elements to figure out how to modify that particular object, before setting it back
	// on the config.
	//
	// Example with the follow configuration:
	//
	//    process_config:
	//      additional_endpoints:
	//        http://url.com:
	//         - ENC[handle_to_password]
	//
	// Calling this function like:
	//
	//   configAssignAtPath(config, ['process_config', 'additional_endpoints', 'http://url.com', '0'], 'password')
	//
	// This is split into:
	//   ['process_config', 'additional_endpoints']  // a known config field
	// and:
	//   ['http://url.com', '0']                     // trailing elements
	//
	// This function will effectively do:
	//
	// var original map[string][]string = config.Get('process_config.additional_endpoints')
	// var slice []string               = original['http://url.com']
	// slice[0] = 'password'
	// config.Set('process_config.additional_endpoints', original)

	trailingElements := make([]string, 0, len(settingPath))
	// copy the path and hold onto the original, useful for error messages
	path := slices.Clone(settingPath)
	for {
		if len(path) == 0 {
			return fmt.Errorf("unknown config setting '%s'", settingPath)
		}
		// get the last element from the path and add it to the trailing elements
		lastElem := path[len(path)-1]
		trailingElements = append(trailingElements, lastElem)
		// remove that element from the path and see if we've reached a known field
		path = path[:len(path)-1]
		settingName = strings.Join(path, ".")
		if config.IsKnown(settingName) {
			break
		}
	}
	slices.Reverse(trailingElements)

	// retrieve the config value at the known field
	startingValue := config.Get(settingName)
	iterateValue := startingValue
	// iterate down until we find the final object that we are able to modify
	for k, elem := range trailingElements {
		switch modifyValue := iterateValue.(type) {
		case map[string]interface{}:
			if k == len(trailingElements)-1 {
				// if we reached the final object, modify it directly by assigning the newValue parameter
				modifyValue[elem] = newValue
			} else {
				// otherwise iterate inside that compound object
				iterateValue = modifyValue[elem]
			}
		case map[interface{}]interface{}:
			if k == len(trailingElements)-1 {
				modifyValue[elem] = newValue
			} else {
				iterateValue = modifyValue[elem]
			}
		case []string:
			index, err := strconv.Atoi(elem)
			if err != nil {
				return err
			}
			if index >= len(modifyValue) {
				return fmt.Errorf("index out of range %d >= %d", index, len(modifyValue))
			}
			if k == len(trailingElements)-1 {
				modifyValue[index] = fmt.Sprintf("%s", newValue)
			} else {
				iterateValue = modifyValue[index]
			}
		case []interface{}:
			index, err := strconv.Atoi(elem)
			if err != nil {
				return err
			}
			if index >= len(modifyValue) {
				return fmt.Errorf("index out of range %d >= %d", index, len(modifyValue))
			}
			if k == len(trailingElements)-1 {
				modifyValue[index] = newValue
			} else {
				iterateValue = modifyValue[index]
			}
		default:
			return fmt.Errorf("cannot assign to setting '%s' of type %T", settingPath, iterateValue)
		}
	}

	config.Set(settingName, startingValue, pkgconfigmodel.SourceAgentRuntime)
	return nil
}

// EnvVarAreSetAndNotEqual returns true if two given variables are set in environment and are not equal.
func EnvVarAreSetAndNotEqual(lhsName string, rhsName string) bool {
	lhsValue, lhsIsSet := os.LookupEnv(lhsName)
	rhsValue, rhsIsSet := os.LookupEnv(rhsName)

	return lhsIsSet && rhsIsSet && lhsValue != rhsValue
}

// sanitizeAPIKeyConfig strips newlines and other control characters from a given key.
func sanitizeAPIKeyConfig(config pkgconfigmodel.Config, key string) {
	if !config.IsKnown(key) || !config.IsSet(key) {
		return
	}
	config.Set(key, strings.TrimSpace(config.GetString(key)), config.GetSource(key))
}

// sanitizeExternalMetricsProviderChunkSize ensures the value of `external_metrics_provider.chunk_size` is within an acceptable range
func sanitizeExternalMetricsProviderChunkSize(config pkgconfigmodel.Config) {
	if !config.IsKnown("external_metrics_provider.chunk_size") {
		return
	}

	chunkSize := config.GetInt("external_metrics_provider.chunk_size")
	if chunkSize <= 0 {
		log.Warnf("external_metrics_provider.chunk_size cannot be negative: %d", chunkSize)
		config.Set("external_metrics_provider.chunk_size", 1, pkgconfigmodel.SourceAgentRuntime)
	}
	if chunkSize > maxExternalMetricsProviderChunkSize {
		log.Warnf("external_metrics_provider.chunk_size has been set to %d, which is higher than the maximum allowed value %d. Using %d.", chunkSize, maxExternalMetricsProviderChunkSize, maxExternalMetricsProviderChunkSize)
		config.Set("external_metrics_provider.chunk_size", maxExternalMetricsProviderChunkSize, pkgconfigmodel.SourceAgentRuntime)
	}
}

func bindEnvAndSetLogsConfigKeys(config pkgconfigmodel.Setup, prefix string) {
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
	config.BindEnvAndSetDefault(prefix+"input_chan_size", DefaultInputChanSize) // Only used by EP Forwarder for now, not used by logs
	config.BindEnvAndSetDefault(prefix+"sender_backoff_factor", DefaultLogsSenderBackoffFactor)
	config.BindEnvAndSetDefault(prefix+"sender_backoff_base", DefaultLogsSenderBackoffBase)
	config.BindEnvAndSetDefault(prefix+"sender_backoff_max", DefaultLogsSenderBackoffMax)
	config.BindEnvAndSetDefault(prefix+"sender_recovery_interval", DefaultForwarderRecoveryInterval)
	config.BindEnvAndSetDefault(prefix+"sender_recovery_reset", false)
	config.BindEnvAndSetDefault(prefix+"use_v2_api", true)
	config.SetKnown(prefix + "dev_mode_no_ssl")
}

// IsCloudProviderEnabled checks the cloud provider family provided in
// pkg/util/<cloud_provider>.go against the value for cloud_provider: on the
// global config object Datadog
func IsCloudProviderEnabled(cloudProviderName string, config pkgconfigmodel.Reader) bool {
	cloudProviderFromConfig := config.GetStringSlice("cloud_provider_metadata")

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

// pathExists returns true if the given path exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// setTracemallocEnabled is a helper to get the effective tracemalloc
// configuration.
func setTracemallocEnabled(config pkgconfigmodel.Config) bool {
	if !config.IsKnown("tracemalloc_debug") {
		return false
	}

	pyVersion := config.GetString("python_version")
	wTracemalloc := config.GetBool("tracemalloc_debug")
	traceMallocEnabledWithPy2 := false
	if pyVersion == "2" && wTracemalloc {
		log.Warnf("Tracemalloc was enabled but unavailable with python version %q, disabling.", pyVersion)
		traceMallocEnabledWithPy2 = true

		// update config with the actual effective tracemalloc
		config.Set("tracemalloc_debug", false, pkgconfigmodel.SourceAgentRuntime)
	}

	return traceMallocEnabledWithPy2
}

// setNumWorkers is a helper to set the effective number of workers for
// a given config.
func setNumWorkers(config pkgconfigmodel.Config) {
	if !config.IsKnown("check_runners") {
		return
	}

	wTracemalloc := config.GetBool("tracemalloc_debug")
	if wTracemalloc {
		log.Infof("Tracemalloc enabled, only one check runner enabled to run checks serially")

		// update config with the actual effective number of workers
		config.Set("check_runners", 1, pkgconfigmodel.SourceAgentRuntime)
	}
}

// IsCLCRunner returns whether the Agent is in cluster check runner mode
func IsCLCRunner(config pkgconfigmodel.Reader) bool {
	if !config.GetBool("clc_runner_enabled") {
		return false
	}

	var cps []ConfigurationProviders
	if err := config.UnmarshalKey("config_providers", &cps); err != nil {
		return false
	}

	for _, name := range config.GetStringSlice("extra_config_providers") {
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
func GetBindHost(config pkgconfigmodel.Reader) string {
	return GetBindHostFromConfig(config)
}

// GetBindHostFromConfig returns the bind_host value from the config
func GetBindHostFromConfig(cfg pkgconfigmodel.Reader) string {
	if cfg.IsSet("bind_host") {
		return cfg.GetString("bind_host")
	}
	return "localhost"
}

// GetValidHostAliases validates host aliases set in `host_aliases` variable and returns
// only valid ones.
func GetValidHostAliases(_ context.Context, config pkgconfigmodel.Reader) ([]string, error) {
	return getValidHostAliasesWithConfig(config), nil
}

func getValidHostAliasesWithConfig(config pkgconfigmodel.Reader) []string {
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

func bindVectorOptions(config pkgconfigmodel.Setup, datatype DataType) {
	config.BindEnvAndSetDefault(fmt.Sprintf("observability_pipelines_worker.%s.enabled", datatype), false)
	config.BindEnvAndSetDefault(fmt.Sprintf("observability_pipelines_worker.%s.url", datatype), "")

	config.BindEnvAndSetDefault(fmt.Sprintf("vector.%s.enabled", datatype), false)
	config.BindEnvAndSetDefault(fmt.Sprintf("vector.%s.url", datatype), "")
}

// GetObsPipelineURL returns the URL under the 'observability_pipelines_worker.' prefix for the given datatype
func GetObsPipelineURL(datatype DataType, config pkgconfigmodel.Reader) (string, error) {
	if config.GetBool(fmt.Sprintf("observability_pipelines_worker.%s.enabled", datatype)) {
		return getObsPipelineURLForPrefix(datatype, "observability_pipelines_worker", config)
	} else if config.GetBool(fmt.Sprintf("vector.%s.enabled", datatype)) {
		// Fallback to the `vector` config if observability_pipelines_worker is not set.
		return getObsPipelineURLForPrefix(datatype, "vector", config)
	}
	return "", nil
}

func getObsPipelineURLForPrefix(datatype DataType, prefix string, config pkgconfigmodel.Reader) (string, error) {
	if config.GetBool(fmt.Sprintf("%s.%s.enabled", prefix, datatype)) {
		pipelineURL := config.GetString(fmt.Sprintf("%s.%s.url", prefix, datatype))
		if pipelineURL == "" {
			log.Errorf("%s.%s.enabled is set to true, but %s.%s.url is empty", prefix, datatype, prefix, datatype)
			return "", nil
		}
		_, err := url.Parse(pipelineURL)
		if err != nil {
			return "", fmt.Errorf("could not parse %s %s endpoint: %s", prefix, datatype, err)
		}
		return pipelineURL, nil
	}
	return "", nil
}

// IsRemoteConfigEnabled returns true if Remote Configuration should be enabled
func IsRemoteConfigEnabled(cfg pkgconfigmodel.Reader) bool {
	// Disable Remote Config for GovCloud
	if cfg.GetBool("fips.enabled") || cfg.GetString("site") == "ddog-gov.com" {
		return false
	}
	return cfg.GetBool("remote_configuration.enabled")
}

// GetRemoteConfigurationAllowedIntegrations returns the list of integrations that can be scheduled
// with remote-config
func GetRemoteConfigurationAllowedIntegrations(cfg pkgconfigmodel.Reader) map[string]bool {
	allowList := cfg.GetStringSlice("remote_configuration.agent_integrations.allow_list")
	allowMap := map[string]bool{}
	for _, integration := range allowList {
		allowMap[strings.ToLower(integration)] = true
	}

	blockList := cfg.GetStringSlice("remote_configuration.agent_integrations.block_list")
	for _, blockedIntegration := range blockList {
		allowMap[strings.ToLower(blockedIntegration)] = false
	}

	return allowMap
}

// IsAgentTelemetryEnabled returns true if Agent Telemetry ise enabled
func IsAgentTelemetryEnabled(cfg pkgconfigmodel.Reader) bool {
	// Disable Agent Telemetry for GovCloud
	if cfg.GetBool("fips.enabled") || cfg.GetString("site") == "ddog-gov.com" {
		return false
	}
	return cfg.GetBool("agent_telemetry.enabled")
}
