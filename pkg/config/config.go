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

	"github.com/spf13/viper"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// DefaultForwarderRecoveryInterval is the default recovery interval, also used if
// the user-provided value is invalid.
const DefaultForwarderRecoveryInterval = 2

// Datadog is the global configuration object
var (
	Datadog = viper.New()
	proxies *Proxy
)

// MetadataProviders helps unmarshalling `metadata_providers` config param
type MetadataProviders struct {
	Name     string        `mapstructure:"name"`
	Interval time.Duration `mapstructure:"interval"`
}

// ConfigurationProviders helps unmarshalling `config_providers` config param
type ConfigurationProviders struct {
	Name        string `mapstructure:"name"`
	Polling     bool   `mapstructure:"polling"`
	TemplateURL string `mapstructure:"template_url"`
	TemplateDir string `mapstructure:"template_dir"`
	Username    string `mapstructure:"username"`
	Password    string `mapstructure:"password"`
	CAFile      string `mapstructure:"ca_file"`
	CAPath      string `mapstructure:"ca_path"`
	CertFile    string `mapstructure:"cert_file"`
	KeyFile     string `mapstructure:"key_file"`
	Token       string `mapstructure:"token"`
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
	// config identifiers
	Datadog.SetConfigName("datadog")
	Datadog.SetEnvPrefix("DD")
	Datadog.SetTypeByDefaultValue(true)

	// Replace '.' from config keys with '_' in env variables bindings.
	// e.g. : BindEnv("foo.bar") will bind config key
	// "foo.bar" to env variable "FOO_BAR"
	Datadog.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Configuration defaults
	// Agent
	BindEnvAndSetDefault("dd_url", "https://app.datadoghq.com")
	BindEnvAndSetDefault("app_key", "")
	Datadog.SetDefault("proxy", nil)
	BindEnvAndSetDefault("skip_ssl_validation", false)
	BindEnvAndSetDefault("hostname", "")
	BindEnvAndSetDefault("tags", []string{})
	BindEnvAndSetDefault("tag_value_split_separator", map[string]string{})
	BindEnvAndSetDefault("conf_path", ".")
	BindEnvAndSetDefault("confd_path", defaultConfdPath)
	BindEnvAndSetDefault("additional_checksd", defaultAdditionalChecksPath)
	BindEnvAndSetDefault("log_payloads", false)
	BindEnvAndSetDefault("log_file", "")
	BindEnvAndSetDefault("log_level", "info")
	BindEnvAndSetDefault("log_to_syslog", false)
	BindEnvAndSetDefault("log_to_console", true)
	BindEnvAndSetDefault("logging_frequency", int64(20))
	BindEnvAndSetDefault("disable_file_logging", false)
	BindEnvAndSetDefault("syslog_uri", "")
	BindEnvAndSetDefault("syslog_rfc", false)
	BindEnvAndSetDefault("syslog_pem", "")
	BindEnvAndSetDefault("syslog_key", "")
	BindEnvAndSetDefault("syslog_tls_verify", true)
	BindEnvAndSetDefault("cmd_host", "localhost")
	BindEnvAndSetDefault("cmd_port", 5001)
	BindEnvAndSetDefault("cluster_agent.cmd_port", 5005)
	BindEnvAndSetDefault("default_integration_http_timeout", 9)
	BindEnvAndSetDefault("enable_metadata_collection", true)
	BindEnvAndSetDefault("enable_gohai", true)
	BindEnvAndSetDefault("check_runners", int64(1))
	BindEnvAndSetDefault("auth_token_file_path", "")
	BindEnvAndSetDefault("bind_host", "localhost")
	BindEnvAndSetDefault("hostname_fqdn", false)
	BindEnvAndSetDefault("cluster_name", "")

	// secrets backend
	Datadog.BindEnv("secret_backend_command")
	Datadog.BindEnv("secret_backend_arguments")
	BindEnvAndSetDefault("secret_backend_output_max_size", 1024)
	BindEnvAndSetDefault("secret_backend_timeout", 5)

	// Retry settings
	BindEnvAndSetDefault("forwarder_backoff_factor", 2)
	BindEnvAndSetDefault("forwarder_backoff_base", 2)
	BindEnvAndSetDefault("forwarder_backoff_max", 64)
	BindEnvAndSetDefault("forwarder_recovery_interval", DefaultForwarderRecoveryInterval)
	BindEnvAndSetDefault("forwarder_recovery_reset", false)

	// Use to output logs in JSON format
	BindEnvAndSetDefault("log_format_json", false)

	// IPC API server timeout
	BindEnvAndSetDefault("server_timeout", 15)

	// Use to force client side TLS version to 1.2
	BindEnvAndSetDefault("force_tls_12", false)

	// Agent GUI access port
	BindEnvAndSetDefault("GUI_port", defaultGuiPort)
	if IsContainerized() {
		Datadog.SetDefault("procfs_path", "/host/proc")
		Datadog.SetDefault("container_proc_root", "/host/proc")
		Datadog.SetDefault("container_cgroup_root", "/host/sys/fs/cgroup/")
	} else {
		Datadog.SetDefault("container_proc_root", "/proc")
		// for amazon linux the cgroup directory on host is /cgroup/
		// we pick memory.stat to make sure it exists and not empty
		if _, err := os.Stat("/cgroup/memory/memory.stat"); !os.IsNotExist(err) {
			Datadog.SetDefault("container_cgroup_root", "/cgroup/")
		} else {
			Datadog.SetDefault("container_cgroup_root", "/sys/fs/cgroup/")
		}
	}

	Datadog.BindEnv("procfs_path")
	Datadog.BindEnv("container_proc_root")
	Datadog.BindEnv("container_cgroup_root")

	BindEnvAndSetDefault("proc_root", "/proc")
	BindEnvAndSetDefault("histogram_aggregates", []string{"max", "median", "avg", "count"})
	BindEnvAndSetDefault("histogram_percentiles", []string{"0.95"})
	// Serializer
	BindEnvAndSetDefault("use_v2_api.series", false)
	BindEnvAndSetDefault("use_v2_api.events", false)
	BindEnvAndSetDefault("use_v2_api.service_checks", false)
	// Serializer: allow user to blacklist any kind of payload to be sent
	BindEnvAndSetDefault("enable_payloads.events", true)
	BindEnvAndSetDefault("enable_payloads.series", true)
	BindEnvAndSetDefault("enable_payloads.service_checks", true)
	BindEnvAndSetDefault("enable_payloads.sketches", true)
	BindEnvAndSetDefault("enable_payloads.json_to_v1_intake", true)

	// Forwarder
	BindEnvAndSetDefault("forwarder_timeout", 20)
	BindEnvAndSetDefault("forwarder_retry_queue_max_size", 30)
	BindEnvAndSetDefault("forwarder_num_workers", 1)
	// Dogstatsd
	BindEnvAndSetDefault("use_dogstatsd", true)
	BindEnvAndSetDefault("dogstatsd_port", 8125)          // Notice: 0 means UDP port closed
	BindEnvAndSetDefault("dogstatsd_buffer_size", 1024*8) // 8KB buffer
	BindEnvAndSetDefault("dogstatsd_non_local_traffic", false)
	BindEnvAndSetDefault("dogstatsd_socket", "") // Notice: empty means feature disabled
	BindEnvAndSetDefault("dogstatsd_stats_port", 5000)
	BindEnvAndSetDefault("dogstatsd_stats_enable", false)
	BindEnvAndSetDefault("dogstatsd_stats_buffer", 10)
	BindEnvAndSetDefault("dogstatsd_expiry_seconds", 300)
	BindEnvAndSetDefault("dogstatsd_origin_detection", false) // Only supported for socket traffic
	BindEnvAndSetDefault("dogstatsd_so_rcvbuf", 0)
	BindEnvAndSetDefault("statsd_forward_host", "")
	BindEnvAndSetDefault("statsd_forward_port", 0)
	BindEnvAndSetDefault("statsd_metric_namespace", "")
	// Autoconfig
	BindEnvAndSetDefault("autoconf_template_dir", "/datadog/check_configs")
	BindEnvAndSetDefault("exclude_pause_container", true)
	BindEnvAndSetDefault("ac_include", []string{})
	BindEnvAndSetDefault("ac_exclude", []string{})

	// Docker
	BindEnvAndSetDefault("docker_query_timeout", int64(5))
	BindEnvAndSetDefault("docker_labels_as_tags", map[string]string{})
	BindEnvAndSetDefault("docker_env_as_tags", map[string]string{})
	BindEnvAndSetDefault("kubernetes_pod_labels_as_tags", map[string]string{})
	BindEnvAndSetDefault("kubernetes_pod_annotations_as_tags", map[string]string{})
	BindEnvAndSetDefault("kubernetes_node_labels_as_tags", map[string]string{})

	// Kubernetes
	BindEnvAndSetDefault("kubernetes_kubelet_host", "")
	BindEnvAndSetDefault("kubernetes_http_kubelet_port", 10255)
	BindEnvAndSetDefault("kubernetes_https_kubelet_port", 10250)

	BindEnvAndSetDefault("kubelet_tls_verify", true)
	BindEnvAndSetDefault("collect_kubernetes_events", false)
	BindEnvAndSetDefault("kubelet_client_ca", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")

	BindEnvAndSetDefault("kubelet_auth_token_path", "")
	BindEnvAndSetDefault("kubelet_client_crt", "")
	BindEnvAndSetDefault("kubelet_client_key", "")

	BindEnvAndSetDefault("kubernetes_collect_metadata_tags", true)
	BindEnvAndSetDefault("kubernetes_metadata_tag_update_freq", 60) // Polling frequency of the Agent to the DCA in seconds (gets the local cache if the DCA is disabled)
	BindEnvAndSetDefault("kubernetes_apiserver_client_timeout", 10)
	BindEnvAndSetDefault("kubernetes_map_services_on_ip", false) // temporary opt-out of the new mapping logic

	// Kube ApiServer
	BindEnvAndSetDefault("kubernetes_kubeconfig_path", "")
	BindEnvAndSetDefault("leader_lease_duration", "60")
	BindEnvAndSetDefault("leader_election", false)
	BindEnvAndSetDefault("kube_resources_namespace", "")

	// Datadog cluster agent
	BindEnvAndSetDefault("cluster_agent.enabled", false)
	BindEnvAndSetDefault("cluster_agent.auth_token", "")
	BindEnvAndSetDefault("cluster_agent.url", "")
	BindEnvAndSetDefault("cluster_agent.kubernetes_service_name", "datadog-cluster-agent")

	// ECS
	BindEnvAndSetDefault("ecs_agent_url", "") // Will be autodetected
	BindEnvAndSetDefault("collect_ec2_tags", false)

	// GCE
	BindEnvAndSetDefault("collect_gce_tags", true)

	// Cloud Foundry
	BindEnvAndSetDefault("cloud_foundry", false)
	BindEnvAndSetDefault("bosh_id", "")

	// JMXFetch
	BindEnvAndSetDefault("jmx_custom_jars", []string{})
	BindEnvAndSetDefault("jmx_use_cgroup_memory_limit", false)

	// Go_expvar server port
	BindEnvAndSetDefault("expvar_port", "5000")

	// Trace agent
	BindEnvAndSetDefault("apm_config.enabled", true)

	// Logs Agent

	// External Use: modify those parameters to configure the logs-agent.
	// enable the logs-agent:
	BindEnvAndSetDefault("logs_enabled", false)
	BindEnvAndSetDefault("log_enabled", false) // deprecated, use logs_enabled instead
	// collect all logs from all containers:
	BindEnvAndSetDefault("logs_config.container_collect_all", false)
	// collect all logs forwarded by TCP on a specific port:
	BindEnvAndSetDefault("logs_config.tcp_forward_port", -1)
	// add a socks5 proxy:
	BindEnvAndSetDefault("logs_config.socks5_proxy_address", "")
	// send the logs to a proxy:
	BindEnvAndSetDefault("logs_config.logs_dd_url", "") // must respect format '<HOST>:<PORT>' and '<PORT>' to be an integer
	BindEnvAndSetDefault("logs_config.logs_no_ssl", false)
	// send the logs to the port 443 of the logs-backend via TCP:
	BindEnvAndSetDefault("logs_config.use_port_443", false)
	// increase the read buffer size of the UDP sockets:
	BindEnvAndSetDefault("logs_config.frame_size", 9000)
	// increase the number of files that can be tailed in parallel:
	BindEnvAndSetDefault("logs_config.open_files_limit", 100)

	// Internal Use Only: avoid modifying those configuration parameters, this could lead to unexpected results.
	BindEnvAndSetDefault("logset", "")
	BindEnvAndSetDefault("logs_config.run_path", defaultRunPath)
	BindEnvAndSetDefault("logs_config.dd_url", "agent-intake.logs.datadoghq.com")
	BindEnvAndSetDefault("logs_config.dd_port", 10516)
	BindEnvAndSetDefault("logs_config.dev_mode_use_proto", true)
	BindEnvAndSetDefault("logs_config.dd_url_443", "agent-443-intake.logs.datadoghq.com")

	// Tagger full cardinality mode
	// Undocumented opt-in feature for now
	BindEnvAndSetDefault("full_cardinality_tagging", false)

	BindEnvAndSetDefault("histogram_copy_to_distribution", false)
	BindEnvAndSetDefault("histogram_copy_to_distribution_prefix", "")

	Datadog.BindEnv("api_key")

	BindEnvAndSetDefault("hpa_watcher_polling_freq", 10)
	BindEnvAndSetDefault("hpa_watcher_gc_period", 60*5) // 5 minutes
	BindEnvAndSetDefault("external_metrics_provider.enabled", false)
	BindEnvAndSetDefault("hpa_configmap_name", "datadog-custom-metrics")
	BindEnvAndSetDefault("external_metrics_provider.refresh_period", 30)
	BindEnvAndSetDefault("external_metrics_provider.batch_window", 5) // 5 seconds to batch calls to the configmap persistent store (GlobalStore)
	BindEnvAndSetDefault("external_metrics_provider.max_age", 60)
	BindEnvAndSetDefault("external_metrics_provider.bucket_size", 60*5) // Window of the metric from Datadog
	BindEnvAndSetDefault("kubernetes_informers_resync_period", 60*5)    // 5 minutes
	BindEnvAndSetDefault("kubernetes_informers_restclient_timeout", 60) // 1 minute

	// Cluster check Autodiscovery
	BindEnvAndSetDefault("cluster_checks.enabled", false)

	setAssetFs()
}

// BindEnvAndSetDefault sets the default value for a config parameter, and adds an env binding
func BindEnvAndSetDefault(key string, val interface{}) {
	Datadog.SetDefault(key, val)
	Datadog.BindEnv(key)
}

var (
	ddURLs = map[string]interface{}{
		"app.datadoghq.com": nil,
		"app.datad0g.com":   nil,
	}
)

// GetProxies returns the proxy settings from the configuration
func GetProxies() *Proxy {
	return proxies
}

// loadProxyFromEnv overrides the proxy settings with environment variables
func loadProxyFromEnv() {
	// Viper doesn't handle mixing nested variables from files and set
	// manually.  If we manually set one of the sub value for "proxy" all
	// other values from the conf file will be shadowed when using
	// 'Datadog.Get("proxy")'. For that reason we first get the value from
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
	if isSet = Datadog.IsSet("proxy"); isSet {
		if err := Datadog.UnmarshalKey("proxy", p); err != nil {
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

	// We have to set each value individually so both Datadog.Get("proxy")
	// and Datadog.Get("proxy.http") work
	if isSet {
		Datadog.Set("proxy.http", p.HTTP)
		Datadog.Set("proxy.https", p.HTTPS)
		Datadog.Set("proxy.no_proxy", p.NoProxy)
		proxies = p
	}
}

// Load reads configs files and initializes the config module
func Load() error {
	log.Infof("config.Load()")
	if err := Datadog.ReadInConfig(); err != nil {
		log.Warnf("confrig.load() error %v", err)
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

	loadProxyFromEnv()
	sanitizeAPIKey()
	return nil
}

// Avoid log ingestion breaking because of a newline in the API key
func sanitizeAPIKey() {
	Datadog.Set("api_key", strings.TrimSpace(Datadog.GetString("api_key")))
}

// GetMultipleEndpoints returns the api keys per domain specified in the main agent config
func GetMultipleEndpoints() (map[string][]string, error) {
	return getMultipleEndpoints(Datadog)
}

// getDomainPrefix provides the right prefix for agent X.Y.Z
func getDomainPrefix(app string) string {
	v, _ := version.New(version.AgentVersion, version.Commit)
	return fmt.Sprintf("%d-%d-%d-%s.agent", v.Major, v.Minor, v.Patch, app)
}

// AddAgentVersionToDomain prefix the domain with the agent version: X-Y-Z.domain
func AddAgentVersionToDomain(domain string, app string) (string, error) {
	u, err := url.Parse(domain)
	if err != nil {
		return "", err
	}

	// we don't udpdate unknown URL (ie: proxy or custom StatsD server)
	if _, found := ddURLs[u.Host]; !found {
		return domain, nil
	}

	subdomain := strings.Split(u.Host, ".")[0]
	newSubdomain := getDomainPrefix(app)

	u.Host = strings.Replace(u.Host, subdomain, newSubdomain, 1)
	return u.String(), nil
}

// getMultipleEndpoints implements the logic to extract the api keys per domain from an agent config
func getMultipleEndpoints(config *viper.Viper) (map[string][]string, error) {
	ddURL := config.GetString("dd_url")

	// Validating domain
	_, err := url.Parse(ddURL)
	if err != nil {
		return nil, fmt.Errorf("Could not parse 'dd_url': %s", err)
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
			return nil, fmt.Errorf("Could not parse url from 'additional_endpoints' %s: %s", domain, err)
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
