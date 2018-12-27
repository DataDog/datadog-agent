// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// DefaultForwarderRecoveryInterval is the default recovery interval, also used if
// the user-provided value is invalid.
const DefaultForwarderRecoveryInterval = 2

// DefaultSite is the default site the Agent sends data to.
const DefaultSite = "datadoghq.com"

const infraURLPrefix = "https://app."

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
	config.BindEnvAndSetDefault("log_level", "info")
	config.BindEnvAndSetDefault("log_to_syslog", false)
	config.BindEnvAndSetDefault("log_to_console", true)
	config.BindEnvAndSetDefault("logging_frequency", int64(20))
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

	// if/when the default is changed to true, make the default platform
	// dependent; default should remain false on Windows to maintain backward
	// compatibility with Agent5 behavior/win
	config.BindEnvAndSetDefault("hostname_fqdn", false)
	config.BindEnvAndSetDefault("cluster_name", "")

	// secrets backend
	config.BindEnv("secret_backend_command")
	config.BindEnv("secret_backend_arguments")
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
	config.BindEnvAndSetDefault("dogstatsd_port", 8125)          // Notice: 0 means UDP port closed
	config.BindEnvAndSetDefault("dogstatsd_buffer_size", 1024*8) // 8KB buffer
	config.BindEnvAndSetDefault("dogstatsd_non_local_traffic", false)
	config.BindEnvAndSetDefault("dogstatsd_socket", "") // Notice: empty means feature disabled
	config.BindEnvAndSetDefault("dogstatsd_stats_port", 5000)
	config.BindEnvAndSetDefault("dogstatsd_stats_enable", false)
	config.BindEnvAndSetDefault("dogstatsd_stats_buffer", 10)
	config.BindEnvAndSetDefault("dogstatsd_expiry_seconds", 300)
	config.BindEnvAndSetDefault("dogstatsd_origin_detection", false) // Only supported for socket traffic
	config.BindEnvAndSetDefault("dogstatsd_so_rcvbuf", 0)
	config.BindEnvAndSetDefault("dogstatsd_tags", []string{})
	config.BindEnvAndSetDefault("statsd_forward_host", "")
	config.BindEnvAndSetDefault("statsd_forward_port", 0)
	config.BindEnvAndSetDefault("statsd_metric_namespace", "")
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

	config.BindEnvAndSetDefault("kubelet_wait_on_missing_container", 0)
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

	// ECS
	config.BindEnvAndSetDefault("ecs_agent_url", "") // Will be autodetected
	config.BindEnvAndSetDefault("ecs_agent_container_name", "ecs-agent")
	config.BindEnvAndSetDefault("collect_ec2_tags", false)

	// GCE
	config.BindEnvAndSetDefault("collect_gce_tags", true)

	// Cloud Foundry
	config.BindEnvAndSetDefault("cloud_foundry", false)
	config.BindEnvAndSetDefault("bosh_id", "")

	// JMXFetch
	config.BindEnvAndSetDefault("jmx_custom_jars", []string{})
	config.BindEnvAndSetDefault("jmx_use_cgroup_memory_limit", false)

	// Go_expvar server port
	config.BindEnvAndSetDefault("expvar_port", "5000")

	// Trace agent
	config.BindEnvAndSetDefault("apm_config.enabled", true)

	// Process agent
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
	config.BindEnvAndSetDefault("logs_config.logs_dd_url", "") // must respect format '<HOST>:<PORT>' and '<PORT>' to be an integer
	config.BindEnvAndSetDefault("logs_config.logs_no_ssl", false)
	// send the logs to the port 443 of the logs-backend via TCP:
	config.BindEnvAndSetDefault("logs_config.use_port_443", false)
	// increase the read buffer size of the UDP sockets:
	config.BindEnvAndSetDefault("logs_config.frame_size", 9000)
	// increase the number of files that can be tailed in parallel:
	config.BindEnvAndSetDefault("logs_config.open_files_limit", 100)

	// Internal Use Only: avoid modifying those configuration parameters, this could lead to unexpected results.
	config.BindEnvAndSetDefault("logset", "")
	config.BindEnvAndSetDefault("logs_config.run_path", defaultRunPath)
	config.BindEnvAndSetDefault("logs_config.dd_url", "agent-intake.logs.datadoghq.com")
	config.BindEnvAndSetDefault("logs_config.dd_port", 10516)
	config.BindEnvAndSetDefault("logs_config.dev_mode_use_proto", true)
	config.BindEnvAndSetDefault("logs_config.dd_url_443", "agent-443-intake.logs.datadoghq.com")
	config.BindEnvAndSetDefault("logs_config.stop_grace_period", 30)

	// Tagger full cardinality mode
	// Undocumented opt-in feature for now
	config.BindEnvAndSetDefault("full_cardinality_tagging", false)

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
	config.BindEnvAndSetDefault("kubernetes_informers_resync_period", 60*5)              // value in seconds. Default to 5 minutes
	config.BindEnvAndSetDefault("kubernetes_informers_restclient_timeout", 60)           // value in seconds
	config.BindEnvAndSetDefault("external_metrics_provider.local_copy_refresh_rate", 30) // value in seconds
	// Cluster check Autodiscovery
	config.BindEnvAndSetDefault("cluster_checks.enabled", false)
	config.BindEnvAndSetDefault("cluster_checks.node_expiration_timeout", 30) // value in seconds

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
	log.Infof("config.Load()")
	if err := Datadog.ReadInConfig(); err != nil {
		log.Warnf("config.load() error %v", err)
		return err
	}
	log.Infof("config.load succeeded")

	// We have to init the secrets package before we can use it to decrypt
	// anything.
	secrets.Init(
		Datadog.GetString("secret_backend_command"),
		Datadog.GetStringSlice("secret_backend_arguments"),
		Datadog.GetInt("secret_backend_timeout"),
		Datadog.GetInt("secret_backend_output_max_size"),
	)

	if Datadog.IsSet("secret_backend_command") {
		// Viper doesn't expose the final location of the file it
		// loads. Since we are searching for 'datadog.yaml' in multiple
		// localtions we let viper determine the one to use before
		// updating it.
		conf, err := yaml.Marshal(Datadog.AllSettings())
		if err != nil {
			return fmt.Errorf("unable to marshal configuration to YAML to decrypt secrets: %v", err)
		}

		finalConfig, err := secrets.Decrypt(conf)
		if err != nil {
			return fmt.Errorf("unable to decrypt secret from datadog.yaml: %v", err)
		}
		r := bytes.NewReader(finalConfig)
		if err = Datadog.MergeConfig(r); err != nil {
			return fmt.Errorf("could not update main configuration after decrypting secrets: %v", err)
		}
	}

	loadProxyFromEnv(Datadog)
	sanitizeAPIKey(Datadog)
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
