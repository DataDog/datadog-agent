// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/spf13/viper"

	"github.com/DataDog/datadog-agent/pkg/version"
)

// Datadog is the global configuration object
var Datadog = viper.New()

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
	Datadog.SetDefault("dd_url", "https://app.datadoghq.com")
	Datadog.SetDefault("app_key", "")
	Datadog.SetDefault("proxy", nil)
	Datadog.SetDefault("skip_ssl_validation", false)
	Datadog.SetDefault("hostname", "")
	Datadog.SetDefault("tags", []string{})
	Datadog.SetDefault("conf_path", ".")
	Datadog.SetDefault("confd_path", defaultConfdPath)
	Datadog.SetDefault("confd_dca_path", defaultDCAConfdPath)
	Datadog.SetDefault("use_service_mapper", true)
	Datadog.SetDefault("additional_checksd", defaultAdditionalChecksPath)
	Datadog.SetDefault("log_payloads", false)
	Datadog.SetDefault("log_level", "info")
	Datadog.SetDefault("log_to_syslog", false)
	Datadog.SetDefault("log_to_console", true)
	Datadog.SetDefault("logging_frequency", int64(20))
	Datadog.SetDefault("disable_file_logging", false)
	Datadog.SetDefault("syslog_uri", "")
	Datadog.SetDefault("syslog_rfc", false)
	Datadog.SetDefault("syslog_tls", false)
	Datadog.SetDefault("syslog_pem", "")
	Datadog.SetDefault("cmd_host", "localhost")
	Datadog.SetDefault("cmd_port", 5001)
	Datadog.SetDefault("clusteragent_cmd_port", 5005)
	Datadog.SetDefault("default_integration_http_timeout", 9)
	Datadog.SetDefault("enable_metadata_collection", true)
	Datadog.SetDefault("enable_gohai", true)
	Datadog.SetDefault("check_runners", int64(1))
	Datadog.SetDefault("expvar_port", "5000")
	Datadog.SetDefault("auth_token_file_path", "")
	Datadog.SetDefault("bind_host", "localhost")

	// Use to output logs in JSON format
	BindEnvAndSetDefault("log_format_json", false)

	// IPC API server timeout
	BindEnvAndSetDefault("server_timeout", 15)

	// Use to force client side TLS version to 1.2
	BindEnvAndSetDefault("force_tls_12", false)

	// Agent GUI access port
	Datadog.SetDefault("GUI_port", defaultGuiPort)
	if IsContainerized() {
		Datadog.SetDefault("container_proc_root", "/host/proc")
		Datadog.SetDefault("procfs_path", "/host/proc")
		Datadog.SetDefault("container_cgroup_root", "/host/sys/fs/cgroup/")
		Datadog.BindEnv("procfs_path")
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
	Datadog.SetDefault("proc_root", "/proc")
	Datadog.SetDefault("histogram_aggregates", []string{"max", "median", "avg", "count"})
	Datadog.SetDefault("histogram_percentiles", []string{"0.95"})
	// Serializer
	Datadog.SetDefault("use_v2_api.series", false)
	Datadog.SetDefault("use_v2_api.events", false)
	Datadog.SetDefault("use_v2_api.service_checks", false)
	// Forwarder
	Datadog.SetDefault("forwarder_timeout", 20)
	Datadog.SetDefault("forwarder_retry_queue_max_size", 30)
	BindEnvAndSetDefault("forwarder_num_workers", 1)
	// Dogstatsd
	Datadog.SetDefault("use_dogstatsd", true)
	Datadog.SetDefault("dogstatsd_port", 8125)          // Notice: 0 means UDP port closed
	Datadog.SetDefault("dogstatsd_buffer_size", 1024*8) // 8KB buffer
	Datadog.SetDefault("dogstatsd_non_local_traffic", false)
	Datadog.SetDefault("dogstatsd_socket", "") // Notice: empty means feature disabled
	Datadog.SetDefault("dogstatsd_stats_port", 5000)
	Datadog.SetDefault("dogstatsd_stats_enable", false)
	Datadog.SetDefault("dogstatsd_stats_buffer", 10)
	Datadog.SetDefault("dogstatsd_expiry_seconds", 300)
	Datadog.SetDefault("dogstatsd_origin_detection", false) // Only supported for socket traffic
	Datadog.SetDefault("statsd_forward_host", "")
	Datadog.SetDefault("statsd_forward_port", 0)
	BindEnvAndSetDefault("statsd_metric_namespace", "")
	// Autoconfig
	Datadog.SetDefault("autoconf_template_dir", "/datadog/check_configs")
	Datadog.SetDefault("exclude_pause_container", true)
	Datadog.SetDefault("ac_include", []string{})
	Datadog.SetDefault("ac_exclude", []string{})

	// Docker
	Datadog.SetDefault("docker_labels_as_tags", map[string]string{})
	Datadog.SetDefault("docker_env_as_tags", map[string]string{})
	Datadog.SetDefault("kubernetes_pod_labels_as_tags", map[string]string{})
	Datadog.SetDefault("kubernetes_pod_annotations_as_tags", map[string]string{})
	Datadog.SetDefault("kubernetes_node_labels_as_tags", map[string]string{})

	// Kubernetes
	Datadog.SetDefault("kubernetes_http_kubelet_port", 10255)
	Datadog.SetDefault("kubernetes_https_kubelet_port", 10250)

	Datadog.SetDefault("kubelet_tls_verify", true)
	Datadog.SetDefault("kubelet_client_ca", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")

	Datadog.SetDefault("kubelet_auth_token_path", "")
	Datadog.SetDefault("kubelet_client_crt", "")
	Datadog.SetDefault("kubelet_client_key", "")

	Datadog.SetDefault("kubernetes_collect_service_tags", true)
	Datadog.SetDefault("kubernetes_service_tag_update_freq", 60*5) // 5 min

	// Kube ApiServer
	Datadog.SetDefault("kubernetes_kubeconfig_path", "")
	Datadog.SetDefault("leader_lease_duration", "60")
	Datadog.SetDefault("leader_election", false)

	// Datadog cluster agent
	Datadog.SetDefault("cluster_agent.auth_token", "")
	Datadog.SetDefault("cluster_agent.url", "")
	Datadog.SetDefault("cluster_agent.kubernetes_service_name", "dca")

	// ECS
	Datadog.SetDefault("ecs_agent_url", "") // Will be autodetected
	Datadog.SetDefault("collect_ec2_tags", false)

	// Cloud Foundry
	Datadog.SetDefault("cloud_foundry", false)
	Datadog.SetDefault("bosh_id", "")

	// JMXFetch
	BindEnvAndSetDefault("jmx_custom_jars", []string{})
	BindEnvAndSetDefault("jmx_use_cgroup_memory_limit", false)

	// Go_expvar server port
	Datadog.SetDefault("expvar_port", "5000")

	// Trace agent
	Datadog.SetDefault("apm_config.enabled", true)

	// Logs Agent
	BindEnvAndSetDefault("logs_enabled", false)
	BindEnvAndSetDefault("log_enabled", false) // deprecated, use logs_enabled instead
	BindEnvAndSetDefault("logset", "")

	BindEnvAndSetDefault("logs_config.dd_url", "intake.logs.datadoghq.com")
	BindEnvAndSetDefault("logs_config.dd_port", 10516)
	BindEnvAndSetDefault("logs_config.dev_mode_use_proto", false)
	BindEnvAndSetDefault("logs_config.run_path", defaultRunPath)
	BindEnvAndSetDefault("logs_config.open_files_limit", 100)
	BindEnvAndSetDefault("logs_config.container_collect_all", false)

	// Tagger full cardinality mode
	// Undocumented opt-in feature for now
	BindEnvAndSetDefault("full_cardinality_tagging", false)

	// ENV vars bindings
	Datadog.BindEnv("api_key")
	Datadog.BindEnv("dd_url")
	Datadog.BindEnv("app_key")
	Datadog.BindEnv("hostname")
	Datadog.BindEnv("tags")
	Datadog.BindEnv("cmd_port")
	Datadog.BindEnv("conf_path")
	Datadog.BindEnv("enable_metadata_collection")
	Datadog.BindEnv("dogstatsd_port")
	Datadog.BindEnv("bind_host")
	Datadog.BindEnv("proc_root")
	Datadog.BindEnv("container_proc_root")
	Datadog.BindEnv("container_cgroup_root")
	Datadog.BindEnv("dogstatsd_socket")
	Datadog.BindEnv("dogstatsd_stats_port")
	Datadog.BindEnv("dogstatsd_non_local_traffic")
	Datadog.BindEnv("dogstatsd_origin_detection")
	Datadog.BindEnv("check_runners")

	Datadog.BindEnv("log_file")
	Datadog.BindEnv("log_level")
	Datadog.BindEnv("log_to_console")

	Datadog.BindEnv("kubernetes_kubelet_host")
	Datadog.BindEnv("kubernetes_http_kubelet_port")
	Datadog.BindEnv("kubernetes_https_kubelet_port")
	Datadog.BindEnv("kubelet_client_crt")
	Datadog.BindEnv("kubelet_client_key")
	Datadog.BindEnv("collect_kubernetes_events")
	Datadog.BindEnv("kubernetes_collect_service_tags")
	Datadog.BindEnv("kubernetes_service_tag_update_freq")
	Datadog.BindEnv("docker_labels_as_tags")
	Datadog.BindEnv("docker_env_as_tags")
	Datadog.BindEnv("kubernetes_pod_labels_as_tags")
	Datadog.BindEnv("kubernetes_pod_annotations_as_tags")
	Datadog.BindEnv("kubernetes_node_labels_as_tags")
	Datadog.BindEnv("ac_include")
	Datadog.BindEnv("ac_exclude")

	Datadog.BindEnv("cluster_agent.auth_token")
	Datadog.BindEnv("clusteragent_cmd_port")

	Datadog.BindEnv("forwarder_timeout")
	Datadog.BindEnv("forwarder_retry_queue_max_size")
	Datadog.BindEnv("cloud_foundry")
	Datadog.BindEnv("bosh_id")
	Datadog.BindEnv("histogram_aggregates")
	Datadog.BindEnv("histogram_percentiles")
	Datadog.BindEnv("kubernetes_kubeconfig_path")
	Datadog.BindEnv("leader_election")
	Datadog.BindEnv("leader_lease_duration")
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

// GetMultipleEndpoints returns the api keys per domain specified in the main agent config
func GetMultipleEndpoints() (map[string][]string, error) {
	return getMultipleEndpoints(Datadog)
}

// getDomainPrefix provides the right prefix for agent X.Y.Z
func getDomainPrefix(app string) string {
	v, _ := version.New(version.AgentVersion, version.Commit)
	return fmt.Sprintf("%d-%d-%d-%s.agent", v.Major, v.Minor, v.Patch, app)
}

// addAgentVersionToDomain prefix the domain with the agent version: X-Y-Z.domain
func addAgentVersionToDomain(domain string, app string) (string, error) {
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
	updatedDDUrl, err := addAgentVersionToDomain(ddURL, "app")
	if err != nil {
		return nil, fmt.Errorf("Could not parse 'dd_url': %s", err)
	}

	keysPerDomain := map[string][]string{
		updatedDDUrl: {
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
		updatedDomain, err := addAgentVersionToDomain(domain, "app")
		if err != nil {
			return nil, fmt.Errorf("Could not parse url from 'additional_endpoints' %s: %s", domain, err)
		}

		if _, ok := keysPerDomain[updatedDomain]; ok {
			for _, apiKey := range apiKeys {
				keysPerDomain[updatedDomain] = append(keysPerDomain[updatedDomain], apiKey)
			}
		} else {
			keysPerDomain[updatedDomain] = apiKeys
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
	return os.Getenv("DOCKER_DD_AGENT") == "yes"
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
