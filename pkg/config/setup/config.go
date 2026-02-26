// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package setup defines the configuration of the agent
package setup

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/pkg/config/create"
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	pkgfips "github.com/DataDog/datadog-agent/pkg/fips"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

const (

	// DefaultFingerprintingMaxBytes is the maximum number of bytes that will be used to generate a checksum fingerprint;
	// used in cases where the line to hash is too large or if the fingerprinting maxLines=0
	DefaultFingerprintingMaxBytes = 100000

	// DefaultLinesOrBytesToSkip is the default number of lines (or bytes) to skip when reading a file.
	// Whether we skip lines or bytes is dependent on whether we choose to compute the fingerprint by lines or by bytes.
	DefaultLinesOrBytesToSkip = 0

	// DefaultFingerprintingCount refers to the number of lines or bytes to use for fingerprinting.
	// This option's default is an invalid value(0), and if not configured will be fixed to the appropriate default
	// value based on the configured fingerprint_strategy.
	DefaultFingerprintingCount = 0

	// DefaultFingerprintStrategy is the default strategy for computing the checksum fingerprint.
	// Options are:
	// - "line_checksum": compute the fingerprint by lines
	// - "byte_checksum": compute the fingerprint by bytes
	// - "disabled": disable fingerprinting
	DefaultFingerprintStrategy = "disabled"

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
	DefaultBatchWait = 5.0

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
	DefaultCompressorKind = "zstd"

	// DefaultLogCompressionKind is the default log compressor. Options available are 'zstd' and 'gzip'
	DefaultLogCompressionKind = "zstd"

	// DefaultZstdCompressionLevel is the default compression level for `zstd`.
	// Compression level 1 provides the lowest compression ratio, but uses much less RSS especially
	// in situations where we have a high value for `GOMAXPROCS`.
	DefaultZstdCompressionLevel = 1

	// DefaultGzipCompressionLevel is the default gzip compression level for logs.
	DefaultGzipCompressionLevel = 6

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
	DefaultMaxMessageSizeBytes = 900 * 1000

	// DefaultNetworkPathTimeout defines the default timeout for a network path test
	DefaultNetworkPathTimeout = 1000

	// DefaultNetworkPathMaxTTL defines the default maximum TTL for traceroute tests
	DefaultNetworkPathMaxTTL = 30

	// DefaultNetworkPathStaticPathTracerouteQueries defines the default number of traceroute queries for static path
	DefaultNetworkPathStaticPathTracerouteQueries = 3

	// DefaultNetworkPathStaticPathE2eQueries defines the default number of end-to-end queries for static path
	DefaultNetworkPathStaticPathE2eQueries = 50
)

var (
	// datadog is the global configuration object
	// NOTE: The constructor `create.New` returns a `model.BuildableConfig`, which is the
	// most general interface for the methods implemented by these types. However, we store
	// them as `model.Config` because that is what the global `Datadog()` accessor returns.
	// Keeping these types aligned signficantly reduces the compiled size of this binary.
	// See https://datadoghq.atlassian.net/wiki/spaces/ACFG/pages/5386798973/Datadog+global+accessor+PR+size+increase
	datadog     pkgconfigmodel.Config
	systemProbe pkgconfigmodel.Config

	datadogMutex     = sync.RWMutex{}
	systemProbeMutex = sync.RWMutex{}
)

// SetDatadog sets the the reference to the agent configuration.
// This is currently used by the legacy converter and config mocks and should not be user anywhere else. Once the
// legacy converter and mock have been migrated we will remove this function.
func SetDatadog(cfg pkgconfigmodel.BuildableConfig) {
	datadogMutex.Lock()
	defer datadogMutex.Unlock()
	datadog = cfg
}

// SetSystemProbe sets the the reference to the systemProbe configuration.
// This is currently used by the config mocks and should not be user anywhere else. Once the mocks have been migrated we
// will remove this function.
func SetSystemProbe(cfg pkgconfigmodel.BuildableConfig) {
	systemProbeMutex.Lock()
	defer systemProbeMutex.Unlock()
	systemProbe = cfg
}

func init() {
	osinit()

	// init default for code that access the config before it initialized
	InitConfigObjects("", "")
}

// Variables to initialize at start time
var (
	// StartTime is the agent startup time
	StartTime = time.Now()
)

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

const (
	// Metrics type covers series & sketches
	Metrics string = "metrics"
	// Logs type covers all outgoing logs
	Logs string = "logs"
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
	fleet,
	autoscaling,
}

type configLibBackend struct {
	ConfNodeTreeModel string `yaml:"conf_nodetreemodel"`
}

func resolveConfigLibType(cliPath string, defaultDir string) string {
	configPath := ""
	for _, path := range []string{cliPath, defaultDir} {
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			path = filepath.Join(path, "datadog.yaml")
		}

		if _, err := os.Stat(path); err == nil {
			configPath = path
		}
	}

	if configPath == "" {
		return ""
	}

	yamlFile, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	conf := configLibBackend{}
	err = yaml.Unmarshal(yamlFile, &conf)
	if err != nil {
		return ""
	}
	return conf.ConfNodeTreeModel
}

// InitConfigObjects initializes the global config objects use across the code. This should never be called anywhere
// but from the main.
func InitConfigObjects(cliPath string, defaultDir string) {
	// We first load the configuration to see which config library should be used.
	configLib := resolveConfigLibType(cliPath, defaultDir)

	// Assign the config globals, using locks to make the tests happy
	SetDatadog(create.NewConfig("datadog", configLib))          // nolint: forbidigo // legitimate use of SetDatadog
	SetSystemProbe(create.NewConfig("system-probe", configLib)) // nolint: forbidigo // legitimate use of SetDatadog

	// Configuration defaults
	initConfig()

	datadog.(pkgconfigmodel.BuildableConfig).BuildSchema()
	systemProbe.(pkgconfigmodel.BuildableConfig).BuildSchema()

	log.Infof("config lib used: %s", datadog.GetLibType())
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
	initCoreAgentFull(config)
}

// LoadProxyFromEnv overrides the proxy settings with environment variables
func LoadProxyFromEnv(config pkgconfigmodel.ReaderWriter) {
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
		if err := structure.UnmarshalKey(config, "proxy", p); err != nil {
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
		p.NoProxy = strings.FieldsFunc(noProxy, func(r rune) bool {
			return r == ',' || r == ' '
		}) // comma and space-separated list, consistent with viper and documentation
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
		config.Set("proxy.http", p.HTTP, pkgconfigmodel.SourceAgentRuntime)
		config.Set("proxy.https", p.HTTPS, pkgconfigmodel.SourceAgentRuntime)

		// If this is set to an empty []string, viper will have a type conflict when merging
		// this config during secrets resolution. It unmarshals empty yaml lists to type
		// []interface{}, which will then conflict with type []string and fail to merge.
		noProxy := make([]interface{}, len(p.NoProxy))
		for idx := range p.NoProxy {
			noProxy[idx] = p.NoProxy[idx]
		}
		config.Set("proxy.no_proxy", noProxy, pkgconfigmodel.SourceAgentRuntime)
	}
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
	for _, loadedKey := range loadedKeys {
		if _, found := knownKeys[loadedKey]; !found {
			nestedValue := false
			// If a value is within a known key it is considered known.
			for knownKey := range knownKeys {
				if strings.HasPrefix(loadedKey, knownKey+".") {
					nestedValue = true
					break
				}
			}
			if !nestedValue {
				unknownKeys = append(unknownKeys, loadedKey)
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
		// signals whether or not ADP is enabled (deprecated)
		"DD_ADP_ENABLED": {},
		// trace-loader socket file descriptors
		"DD_APM_NET_RECEIVER_FD":  {},
		"DD_APM_UNIX_RECEIVER_FD": {},
		"DD_OTLP_CONFIG_GRPC_FD":  {},
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

// LoadDatadog reads config files and initializes config with decrypted secrets
func LoadDatadog(config pkgconfigmodel.Config, secretResolver secrets.Component, additionalEnvVars []string) error {
	// Feature detection running in a defer func as it always  need to run (whether config load has been successful or not)
	// Because some Agents (e.g. trace-agent) will run even if config file does not exist
	defer func() {
		// Environment feature detection needs to run before applying override funcs
		// as it may provide such overrides
		pkgconfigenv.DetectFeatures(config)
		pkgconfigmodel.ApplyOverrideFuncs(config)
	}()

	err := loadCustom(config, additionalEnvVars)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return log.Warnf("Error loading config: %v (check config file permissions for dd-agent user)", err)
		}
		return err
	}

	// We resolve proxy setting before secrets. This allows setting secrets through DD_PROXY_* env variables
	LoadProxyFromEnv(config)

	if err := resolveSecrets(config, secretResolver, "datadog.yaml"); err != nil {
		return err
	}

	// Verify 'DD_URL' and 'DD_DD_URL' conflicts
	if envVarAreSetAndNotEqual("DD_DD_URL", "DD_URL") {
		log.Warnf("'DD_URL' and 'DD_DD_URL' variables are both set in environment. Using 'DD_DD_URL' value")
	}

	useHostEtc(config)

	err = checkConflictingOptions(config)
	if err != nil {
		return err
	}

	sanitizeAPIKeyConfig(config, "api_key")
	sanitizeAPIKeyConfig(config, "logs_config.api_key")
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

	return setupFipsEndpoints(config)
}

// LoadSystemProbe reads config files and initializes config with decrypted secrets for system-probe
func LoadSystemProbe(config pkgconfigmodel.Config, additionalKnownEnvVars []string) error {
	return loadCustom(config, additionalKnownEnvVars)
}

// loadCustom reads config into the provided config object
func loadCustom(config pkgconfigmodel.Config, additionalKnownEnvVars []string) error {
	log.Info("Starting to load the configuration")
	if err := config.ReadInConfig(); err != nil {
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

	// The `datadog-fips-agent` flavor is incompatible with the fips-proxy and we do not want to downgrade to http or
	// route traffic through a proxy for the above products
	fipsFlavor, err := pkgfips.Enabled()
	if err != nil {
		return err
	}

	if fipsFlavor {
		log.Debug("FIPS mode is enabled in the agent. Ignoring fips-proxy settings")
		return nil
	}

	if !config.GetBool("fips.enabled") {
		log.Debug("FIPS mode is disabled")
		return nil
	}

	log.Warn("The FIPS Agent (`datadog-fips-agent`) will replace the FIPS Proxy as the FIPS-compliant implementation of the Agent in the future. Please ensure that you transition to `datadog-fips-agent` as soon as possible.")

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
	_ = os.Unsetenv("HTTP_PROXY")
	_ = os.Unsetenv("HTTPS_PROXY")

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

	setupFipsLogsConfig(config, "network_devices.metadata.", urlFor(networkDevicesMetadata))
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

// resolveSecrets merges all the secret values from origin into config. Secret values
// are identified by a value of the form "ENC[key]" where key is the secret key.
// See: https://github.com/DataDog/datadog-agent/blob/main/docs/agent/secrets.md
func resolveSecrets(config pkgconfigmodel.Config, secretResolver secrets.Component, origin string) error {
	log.Info("Starting to resolve secrets")
	// We have to init the secrets package before we can use it to decrypt
	// anything.
	secretResolver.Configure(secrets.ConfigParams{
		Type:                         config.GetString("secret_backend_type"),
		Config:                       config.GetStringMap("secret_backend_config"),
		Command:                      config.GetString("secret_backend_command"),
		Arguments:                    config.GetStringSlice("secret_backend_arguments"),
		Timeout:                      config.GetInt("secret_backend_timeout"),
		MaxSize:                      config.GetInt("secret_backend_output_max_size"),
		RefreshInterval:              config.GetInt("secret_refresh_interval"),
		RefreshIntervalScatter:       config.GetBool("secret_refresh_scatter"),
		GroupExecPerm:                config.GetBool("secret_backend_command_allow_group_exec_perm"),
		RemoveLinebreak:              config.GetBool("secret_backend_remove_trailing_line_break"),
		RunPath:                      config.GetString("run_path"),
		AuditFileMaxSize:             config.GetInt("secret_audit_file_max_size"),
		ScopeIntegrationToNamespace:  config.GetBool("secret_scope_integration_to_their_k8s_namespace"),
		AllowedNamespace:             config.GetStringSlice("secret_allowed_k8s_namespace"),
		ImageToHandle:                config.GetStringMapStringSlice("secret_image_to_handle"),
		APIKeyFailureRefreshInterval: config.GetInt("secret_refresh_on_api_key_failure_interval"),
	})

	if config.GetString("secret_backend_command") != "" || config.GetString("secret_backend_type") != "" {
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
		if _, err = secretResolver.Resolve(yamlConf, origin, "", "", true); err != nil {
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
				// use integer key when it exists in map to avoid mixing string and integer keys (e.g., "2" and 2)
				if index, err := strconv.Atoi(elem); err == nil {
					if _, exists := modifyValue[index]; exists {
						modifyValue[index] = newValue
						continue
					}
				}
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

// envVarAreSetAndNotEqual returns true if two given variables are set in environment and are not equal.
func envVarAreSetAndNotEqual(lhsName string, rhsName string) bool {
	lhsValue, lhsIsSet := os.LookupEnv(lhsName)
	rhsValue, rhsIsSet := os.LookupEnv(rhsName)

	return lhsIsSet && rhsIsSet && lhsValue != rhsValue
}

// sanitizeAPIKeyConfig strips newlines and other control characters from a given key.
func sanitizeAPIKeyConfig(config pkgconfigmodel.Config, key string) {
	if !config.IsKnown(key) || !config.IsConfigured(key) {
		return
	}
	original := config.GetString(key)
	trimmed := strings.TrimSpace(original)
	if original == trimmed {
		return
	}
	config.Set(key, trimmed, pkgconfigmodel.SourceAgentRuntime)
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

func toggleDefaultPayloads(config pkgconfigmodel.Config) {
	// Disables metric data submission (including Custom Metrics) so that hosts stop showing up in Datadog.
	// Used namely for Error Tracking Standalone where it is not needed.
	if !config.GetBool("core_agent.enabled") {
		config.Set("enable_payloads.events", false, pkgconfigmodel.SourceAgentRuntime)
		config.Set("enable_payloads.series", false, pkgconfigmodel.SourceAgentRuntime)
		config.Set("enable_payloads.service_checks", false, pkgconfigmodel.SourceAgentRuntime)
		config.Set("enable_payloads.sketches", false, pkgconfigmodel.SourceAgentRuntime)
	}
}

func applyInfrastructureModeOverrides(config pkgconfigmodel.Config) {
	infraMode := config.GetString("infrastructure_mode")

	// Apply legacy alias: copy values from legacy key to integration.additional
	// Legacy `allowed_additional_checks` -> `integration.additional`
	if legacyAdditional := config.GetStringSlice("allowed_additional_checks"); len(legacyAdditional) > 0 {
		combined := append(config.GetStringSlice("integration.additional"), legacyAdditional...)
		config.Set("integration.additional", combined, pkgconfigmodel.SourceAgentRuntime)
	}

	if infraMode == "end_user_device" {
		defaultNetworkPathCollectorFilters := []map[string]string{
			// Exclude everything by default
			{"match_domain": "*", "type": "exclude"},

			// 1. Microsoft 365 (without IP addresses)
			// https://tinyurl.com/4a3ydsrs
			{"match_domain": "*.aadrm.com", "type": "include"},
			{"match_domain": "*.aka.ms", "type": "include"},
			{"match_domain": "*.amp.azure.net", "type": "include"},
			{"match_domain": "*.apps.mil", "type": "include"},
			{"match_domain": "*.aspnetcdn.com", "type": "include"},
			{"match_domain": "*.azure.com", "type": "include"},
			{"match_domain": "*.azure.net", "type": "include"},
			{"match_domain": "*.azure.us", "type": "include"},
			{"match_domain": "*.azurerms.com", "type": "include"},
			{"match_domain": "*.bing.com", "type": "include"},
			{"match_domain": "*.bing.net", "type": "include"},
			{"match_domain": "*.cloudappsecurity.com", "type": "include"},
			{"match_domain": "*.cortana.ai", "type": "include"},
			{"match_domain": "*.digicert.com", "type": "include"},
			{"match_domain": "*.dps.mil", "type": "include"},
			{"match_domain": "*.entrust.net", "type": "include"},
			{"match_domain": "*.geotrust.com", "type": "include"},
			{"match_domain": "*.globalsign.com", "type": "include"},
			{"match_domain": "*.globalsign.net", "type": "include"},
			{"match_domain": "*.identrust.com", "type": "include"},
			{"match_domain": "*.letsencrypt.org", "type": "include"},
			{"match_domain": "*.linkedin.com", "type": "include"},
			{"match_domain": "*.live.com", "type": "include"},
			{"match_domain": "*.live.net", "type": "include"},
			{"match_domain": "*.microsoft.us", "type": "include"},
			{"match_domain": "*.microsoftazure.us", "type": "include"},
			{"match_domain": "*.microsoftonline.com", "type": "include"},
			{"match_domain": "*.microsoftonline.us", "type": "include"},
			{"match_domain": "*.microsoft", "type": "include"},
			{"match_domain": "*.msecnd.net", "type": "include"},
			{"match_domain": "*.msauth.net", "type": "include"},
			{"match_domain": "*.msauthimages.net", "type": "include"},
			{"match_domain": "*.msedge.net", "type": "include"},
			{"match_domain": "*.msftauth.net", "type": "include"},
			{"match_domain": "*.msocdn.com", "type": "include"},
			{"match_domain": "*.o365weve.com", "type": "include"},
			{"match_domain": "*.office.com", "type": "include"},
			{"match_domain": "*.office.net", "type": "include"},
			{"match_domain": "*.office365.com", "type": "include"},
			{"match_domain": "*.office365.us", "type": "include"},
			{"match_domain": "*.onestore.ms", "type": "include"},
			{"match_domain": "*.onedrive.com", "type": "include"},
			{"match_domain": "*.onenote.com", "type": "include"},
			{"match_domain": "*.onmicrosoft.com", "type": "include"},
			{"match_domain": "*.powerapps.com", "type": "include"},
			{"match_domain": "*.powerautomate.com", "type": "include"},
			{"match_domain": "*.powerplatform.com", "type": "include"},
			{"match_domain": "*.public-trust.com", "type": "include"},
			{"match_domain": "*.sfx.ms", "type": "include"},
			{"match_domain": "*.sharepoint.com", "type": "include"},
			{"match_domain": "*.sharepoint-mil.us", "type": "include"},

			// 2. Google Workspace
			// https://tinyurl.com/tvdmkrpy
			{"match_domain": "*.google.com", "type": "include"},
			{"match_domain": "*.googleapis.com", "type": "include"},
			{"match_domain": "*.googledrive.com", "type": "include"},
			{"match_domain": "*.googleusercontent.com", "type": "include"},
			{"match_domain": "*.gstatic.com", "type": "include"},
			{"match_domain": "*.youtube.com", "type": "include"},

			// 3. Zoom
			// https://tinyurl.com/594954h7
			{"match_domain": "*.zoom.us", "type": "include"},
			{"match_domain": "*.zoom.com", "type": "include"},

			// 4. Slack
			// https://docs.slack.dev/faq
			{"match_domain": "*.slack-core.com", "type": "include"},
			{"match_domain": "*.slack-edge.com", "type": "include"},
			{"match_domain": "*.slack-files.com", "type": "include"},
			{"match_domain": "*.slack-imgs.com", "type": "include"},
			{"match_domain": "*.slack-msgs.com", "type": "include"},
			{"match_domain": "*.slack.com", "type": "include"},
			{"match_domain": "*.slackb.com", "type": "include"},

			// 5. Salesforce
			// https://tinyurl.com/y5jet7cn
			{"match_domain": "*.documentforce.com", "type": "include"},
			{"match_domain": "*.force-user-content.com", "type": "include"},
			{"match_domain": "*.force.com", "type": "include"},
			{"match_domain": "*.forceusercontent.com", "type": "include"},
			{"match_domain": "*.lightning.com", "type": "include"},
			{"match_domain": "*.salesforce-communities.com", "type": "include"},
			{"match_domain": "*.salesforce-experience.com", "type": "include"},
			{"match_domain": "*.salesforce-hub.com", "type": "include"},
			{"match_domain": "*.salesforce-scrt.com", "type": "include"},
			{"match_domain": "*.salesforce-setup.com", "type": "include"},
			{"match_domain": "*.salesforce-sites.com", "type": "include"},
			{"match_domain": "*.salesforce.com", "type": "include"},
			{"match_domain": "*.salesforceiq.com", "type": "include"},
			{"match_domain": "*.salesforceliveagent.com", "type": "include"},
			{"match_domain": "*.sfdc.sh", "type": "include"},
			{"match_domain": "*.sfdcfc.net", "type": "include"},
			{"match_domain": "*.sfdcopens.com", "type": "include"},
			{"match_domain": "*.site.com", "type": "include"},
			{"match_domain": "*.trailblazer.me", "type": "include"},
			{"match_domain": "*.trailhead.com", "type": "include"},

			// 6. ServiceNow
			{"match_domain": "*.service-now.com", "type": "include"},
			{"match_domain": "*.servicenow.com", "type": "include"},
			{"match_domain": "*.servicenowservices.com", "type": "include"},
			{"match_domain": "*.sncustomer.com", "type": "include"},
			{"match_domain": "*.sncustomertest.com", "type": "include"},
			{"match_domain": "*.snhosting.com", "type": "include"},

			// 7. Workday
			{"match_domain": "*.myworkday.com", "type": "include"},
			{"match_domain": "*.myworkdaygadgets.com", "type": "include"},
			{"match_domain": "*.myworkdayjobs.com", "type": "include"},
			{"match_domain": "*.myworkdaysite.com", "type": "include"},
			{"match_domain": "*.workday.com", "type": "include"},

			// 8. Atlassian
			// https://tinyurl.com/2fxexx5h
			{"match_domain": "*.atl-paas.net", "type": "include"},
			{"match_domain": "*.atlassian-dev-us-gov-mod.net", "type": "include"},
			{"match_domain": "*.atlassian-dev.net", "type": "include"},
			{"match_domain": "*.atlassian-us-gov-mod.com", "type": "include"},
			{"match_domain": "*.atlassian-us-gov-mod.net", "type": "include"},
			{"match_domain": "*.atlassian.com", "type": "include"},
			{"match_domain": "*.atlassian.net", "type": "include"},
			{"match_domain": "*.bitbucket.org", "type": "include"},
			{"match_domain": "*.jira.com", "type": "include"},
			{"match_domain": "*.ss-inf.net", "type": "include"},

			// 9. GitHub
			{"match_domain": "*.github.com", "type": "include"},

			// 10. Okta
			// https://tinyurl.com/54rddask
			{"match_domain": "*.okta.com", "type": "include"},
			{"match_domain": "*.okta-emea.com", "type": "include"},
			{"match_domain": "*.okta-gov.com", "type": "include"},
			{"match_domain": "*.okta.mil", "type": "include"},
			{"match_domain": "*.okta-preview.com", "type": "include"},
			{"match_domain": "*.oktapreview.com", "type": "include"},
			{"match_domain": "*.oktacdn.com", "type": "include"},

			// 11. Cisco WebEx
			// https://tinyurl.com/ye9bawcc
			{"match_domain": "*.wbx2.com", "type": "include"},
			{"match_domain": "*.webex.com", "type": "include"},

			// 12. Box
			// https://tinyurl.com/2eyv6wr4
			{"match_domain": "*.box.com", "type": "include"},
			{"match_domain": "*.box.net", "type": "include"},
			{"match_domain": "*.boxcdn.net", "type": "include"},
			{"match_domain": "*.boxcloud.com", "type": "include"},

			// 13. Dropbox
			// https://tinyurl.com/pmne8a73
			{"match_domain": "*.addtodropbox.com", "type": "include"},
			{"match_domain": "*.dash.ai", "type": "include"},
			{"match_domain": "*.db.tt", "type": "include"},
			{"match_domain": "*.docsend.com", "type": "include"},
			{"match_domain": "*.dropbox.com", "type": "include"},
			{"match_domain": "*.dropbox.tech", "type": "include"},
			{"match_domain": "*.dropbox.zendesk.com", "type": "include"},
			{"match_domain": "*.dropboxapi.com", "type": "include"},
			{"match_domain": "*.dropboxbusiness.com", "type": "include"},
			{"match_domain": "*.dropboxcaptcha.com", "type": "include"},
			{"match_domain": "*.dropboxexperiment.com", "type": "include"},
			{"match_domain": "*.dropboxforum.com", "type": "include"},
			{"match_domain": "*.dropboxforums.com", "type": "include"},
			{"match_domain": "*.dropboxinsiders.com", "type": "include"},
			{"match_domain": "*.dropboxlegal.com", "type": "include"},
			{"match_domain": "*.dropboxmail.com", "type": "include"},
			{"match_domain": "*.dropboxpartners.com", "type": "include"},
			{"match_domain": "*.dropboxstatic.com", "type": "include"},
			{"match_domain": "*.dropboxteam.com", "type": "include"},
			{"match_domain": "*.getdropbox.com", "type": "include"},
			{"match_domain": "*.hellofax.com", "type": "include"},
			{"match_domain": "*.hellosign.com", "type": "include"},

			// 14. Monday.com
			{"match_domain": "*.monday.com", "type": "include"},

			// 15. OpenAI/ChatGPT
			// https://tinyurl.com/3ye2uwfj
			{"match_domain": "*.openai.com", "type": "include"},
			{"match_domain": "*.chatgpt.com", "type": "include"},

			// 16. Cursor
			// https://tinyurl.com/y6f85d6d
			{"match_domain": "*.cursor.sh", "type": "include"},
			{"match_domain": "*.cursor-cdn.com", "type": "include"},

			// 17. Anthropic/Claude
			{"match_domain": "anthropic.com", "type": "include"},
			{"match_domain": "claude.ai", "type": "include"},
		}

		// Append user-defined filters to the defaults
		if userFilters := config.Get("network_path.collector.filters"); userFilters != nil {
			if userFiltersList, ok := userFilters.([]interface{}); ok {
				for _, f := range userFiltersList {
					if filterMap, ok := f.(map[string]interface{}); ok {
						converted := make(map[string]string)
						for k, v := range filterMap {
							if strVal, ok := v.(string); ok {
								converted[k] = strVal
							}
						}
						// Always append the user defined filters to the defaults at the end of the list to get the higher priority than the default configuration
						defaultNetworkPathCollectorFilters = append(defaultNetworkPathCollectorFilters, converted)
					}
				}
			}
		}
		config.Set("network_path.collector.filters", defaultNetworkPathCollectorFilters, pkgconfigmodel.SourceAgentRuntime) // Agent runtime source is required to override customer defined filters with default configuration

		// Enable features for end_user_device mode
		config.Set("process_config.process_collection.enabled", true, pkgconfigmodel.SourceInfraMode)
		config.Set("software_inventory.enabled", true, pkgconfigmodel.SourceInfraMode)
		config.Set("notable_events.enabled", true, pkgconfigmodel.SourceInfraMode)
	} else if infraMode == "none" {
		// Disable integrations (no host metrics collection)
		config.Set("integration.enabled", false, pkgconfigmodel.SourceInfraMode)
	}
}

func bindEnvAndSetLogsConfigKeys(config pkgconfigmodel.Setup, prefix string) {
	config.BindEnv(prefix + "logs_dd_url")          //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv' // Send the logs to a proxy. Must respect format '<HOST>:<PORT>' and '<PORT>' to be an integer
	config.BindEnv(prefix + "dd_url")               //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	config.BindEnv(prefix + "additional_endpoints") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	config.BindEnvAndSetDefault(prefix+"use_compression", true)
	config.BindEnvAndSetDefault(prefix+"compression_kind", DefaultLogCompressionKind)
	config.BindEnvAndSetDefault(prefix+"zstd_compression_level", DefaultZstdCompressionLevel) // Default level for the zstd algorithm
	config.BindEnvAndSetDefault(prefix+"compression_level", DefaultGzipCompressionLevel)      // Default level for the gzip algorithm
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
	config.SetDefault(prefix+"dev_mode_no_ssl", false)
}

// pathExists returns true if the given path exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
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
	if err := structure.UnmarshalKey(config, "config_providers", &cps); err != nil {
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
