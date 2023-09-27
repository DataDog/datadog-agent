// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/pkg/conf"
	"github.com/DataDog/datadog-agent/pkg/config/configsetup"
	"github.com/DataDog/datadog-agent/pkg/config/load"
	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	// DefaultLocalProcessCollectorInterval is the interval at which processes are collected and sent to the workloadmeta
	// in the core agent if the process check is disabled.
	DefaultLocalProcessCollectorInterval = 1 * time.Minute

	// DefaultMaxMessageSizeBytes is the default value for max_message_size_bytes
	// If a log message is larger than this byte limit, the overflow bytes will be truncated.
	DefaultMaxMessageSizeBytes = 256 * 1000
)

// Datadog is the global configuration object
var (
	Datadog       Config
	SystemProbe   Config
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

	// DefaultSecurityProfilesDir is the default directory used to store Security Profiles by the runtime security module
	DefaultSecurityProfilesDir = filepath.Join(defaultRunPath, "runtime-security", "profiles")
)

// PrometheusScrapeChecksTransformer unmarshals a prometheus check.
func PrometheusScrapeChecksTransformer(in string) interface{} {
	var promChecks []*types.PrometheusCheck
	if err := json.Unmarshal([]byte(in), &promChecks); err != nil {
		log.Warnf(`"prometheus_scrape.checks" can not be parsed: %v`, err)
	}
	return promChecks
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

// MappingProfile represent a group of mappings
type MappingProfile struct {
	Name     string          `mapstructure:"name" json:"name" yaml:"name"`
	Prefix   string          `mapstructure:"prefix" json:"prefix" yaml:"prefix"`
	Mappings []MetricMapping `mapstructure:"mappings" json:"mappings" yaml:"mappings"`
}

// MetricMapping represent one mapping rule
type MetricMapping struct {
	Match     string            `mapstructure:"match" json:"match" yaml:"match"`
	MatchType string            `mapstructure:"match_type" json:"match_type" yaml:"match_type"`
	Name      string            `mapstructure:"name" json:"name" yaml:"name"`
	Tags      map[string]string `mapstructure:"tags" json:"tags" yaml:"tags"`
}

// Endpoint represent a datadog endpoint
type Endpoint struct {
	Site   string `mapstructure:"site" json:"site" yaml:"site"`
	URL    string `mapstructure:"url" json:"url" yaml:"url"`
	APIKey string `mapstructure:"api_key" json:"api_key" yaml:"api_key"`
	APPKey string `mapstructure:"app_key" json:"app_key" yaml:"app_key"`
}

// Warnings represent the warnings in the config
type Warnings = conf.Warnings

// DataType represent the generic data type (e.g. metrics, logs) that can be sent by the Agent
type DataType = configsetup.DataType

const (
	// Metrics type covers series & sketches
	Metrics = configsetup.Metrics
	// Logs type covers all outgoing logs
	Logs DataType = configsetup.Logs
)

func init() {
	osinit()
	// Configure Datadog global configuration
	Datadog = NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	SystemProbe = NewConfig("system-probe", "DD", strings.NewReplacer(".", "_"))
	// Configuration defaults
	configsetup.InitConfig(Datadog)
	configsetup.InitSystemProbeConfig(SystemProbe)
}

// LoadProxyFromEnv overrides the proxy settings with environment variables
var LoadProxyFromEnv = load.LoadProxyFromEnv

// Load reads configs files and initializes the config module
func Load() (*Warnings, error) {
	return load.Load(Datadog, "datadog.yaml", SystemProbe.GetEnvVars())
}

// LoadWithoutSecret reads configs files, initializes the config module without decrypting any secrets
func LoadWithoutSecret() (*Warnings, error) {
	return load.LoadWithoutSecret(Datadog, "datadog.yaml", SystemProbe.GetEnvVars())
}

// Merge will merge additional configuration into an existing configuration
func Merge(configPaths []string) error {
	return conf.Merge(configPaths, Datadog)
}

func checkConflictingOptions(config Config) error {
	// Verify that either use_podman_logs OR docker_path_override are set since they conflict
	if config.GetBool("logs_config.use_podman_logs") && config.IsSet("logs_config.docker_path_override") {
		log.Warnf("'use_podman_logs' is set to true and 'docker_path_override' is set, please use one or the other")
		return errors.New("'use_podman_logs' is set to true and 'docker_path_override' is set, please use one or the other")
	}

	return nil
}

var LoadDatadogCustom = load.LoadDatadogCustom

// LoadCustom reads config into the provided config object
var LoadCustom = load.LoadCustom

// setupFipsEndpoints overwrites the Agent endpoint for outgoing data to be sent to the local FIPS proxy. The local FIPS
// proxy will be in charge of forwarding data to the Datadog backend following FIPS standard. Starting from
// fips.port_range_start we will assign a dedicated port per product (metrics, logs, traces, ...).

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
		config.GetBool("secret_backend_remove_trailing_line_break"),
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

// sanitizeAPIKeyConfig strips newlines and other control characters from a given key.
func sanitizeAPIKeyConfig(config Config, key string) {
	if !config.IsKnown(key) {
		return
	}
	config.Set(key, strings.TrimSpace(config.GetString(key)))
}

// sanitizeExternalMetricsProviderChunkSize ensures the value of `external_metrics_provider.chunk_size` is within an acceptable range
func sanitizeExternalMetricsProviderChunkSize(config Config) {
	if !config.IsKnown("external_metrics_provider.chunk_size") {
		return
	}

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
	config.BindEnvAndSetDefault(prefix+"input_chan_size", DefaultInputChanSize) // Only used by EP Forwarder for now, not used by logs
	config.BindEnvAndSetDefault(prefix+"sender_backoff_factor", DefaultLogsSenderBackoffFactor)
	config.BindEnvAndSetDefault(prefix+"sender_backoff_base", DefaultLogsSenderBackoffBase)
	config.BindEnvAndSetDefault(prefix+"sender_backoff_max", DefaultLogsSenderBackoffMax)
	config.BindEnvAndSetDefault(prefix+"sender_recovery_interval", DefaultForwarderRecoveryInterval)
	config.BindEnvAndSetDefault(prefix+"sender_recovery_reset", false)
	config.BindEnvAndSetDefault(prefix+"use_v2_api", true)
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

// GetIPCAddress returns the IPC address or an error if the address is not local
func GetIPCAddress() (string, error) {
	return conf.GetIPCAddress(Datadog)
}

// pathExists returns true if the given path exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
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
	return GetBindHostFromConfig(Datadog)
}

func GetBindHostFromConfig(cfg ConfigReader) string {
	if cfg.IsSet("bind_host") {
		return cfg.GetString("bind_host")
	}
	return "localhost"
}

// GetValidHostAliases validates host aliases set in `host_aliases` variable and returns
// only valid ones.
func GetValidHostAliases(_ context.Context) ([]string, error) {
	return getValidHostAliasesWithConfig(Datadog), nil
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

// GetObsPipelineURL returns the URL under the 'observability_pipelines_worker.' prefix for the given datatype
func GetObsPipelineURL(datatype DataType) (string, error) {
	return configsetup.GetObsPipelineURL(datatype, Datadog)
}

// IsRemoteConfigEnabled returns true if Remote Configuration should be enabled
func IsRemoteConfigEnabled(cfg ConfigReader) bool {
	// Disable Remote Config for GovCloud
	if cfg.GetBool("fips.enabled") || cfg.GetString("site") == "ddog-gov.com" {
		return false
	}
	return cfg.GetBool("remote_configuration.enabled")
}

var (
	InitSystemProbeConfig = configsetup.InitSystemProbeConfig
	InitConfig            = configsetup.InitConfig
)
