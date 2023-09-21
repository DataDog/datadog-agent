package load

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/conf"
	"github.com/DataDog/datadog-agent/pkg/conf/env"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const // maxExternalMetricsProviderChunkSize ensures batch queries are limited in size.
maxExternalMetricsProviderChunkSize = 35

// Variables to initialize at build time
var (
	DefaultPython string

	// ForceDefaultPython has its value set to true at compile time if we should ignore
	// the Python version set in the configuration and use `DefaultPython` instead.
	// We use this to force Python 3 in the Agent 7 as it's the only one available.
	ForceDefaultPython string
)

// EnvVarAreSetAndNotEqual returns true if two given variables are set in environment and are not equal.
func EnvVarAreSetAndNotEqual(lhsName string, rhsName string) bool {
	lhsValue, lhsIsSet := os.LookupEnv(lhsName)
	rhsValue, rhsIsSet := os.LookupEnv(rhsName)

	return lhsIsSet && rhsIsSet && lhsValue != rhsValue
}

// sanitizeAPIKeyConfig strips newlines and other control characters from a given key.
func sanitizeAPIKeyConfig(config conf.Config, key string) {
	if !config.IsKnown(key) {
		return
	}
	config.Set(key, strings.TrimSpace(config.GetString(key)))
}

// sanitizeExternalMetricsProviderChunkSize ensures the value of `external_metrics_provider.chunk_size` is within an acceptable range
func sanitizeExternalMetricsProviderChunkSize(config conf.Config) {
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

// setTracemallocEnabled is a helper to get the effective tracemalloc
// configuration.
func setTracemallocEnabled(config conf.Config) bool {
	if !config.IsKnown("tracemalloc_debug") {
		return false
	}

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
func setNumWorkers(config conf.Config) {
	if !config.IsKnown("check_runners") {
		return
	}

	wTracemalloc := config.GetBool("tracemalloc_debug")
	numWorkers := config.GetInt("check_runners")
	if wTracemalloc {
		log.Infof("Tracemalloc enabled, only one check runner enabled to run checks serially")
		numWorkers = 1
	}

	// update config with the actual effective number of workers
	config.Set("check_runners", numWorkers)
}

func findUnknownKeys(config conf.Config) []string {
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

func findUnknownEnvVars(config conf.Config, environ []string, additionalKnownEnvVars []string) []string {
	var unknownVars []string

	knownVars := map[string]struct{}{
		// these variables are used by the agent, but not via the Config struct,
		// so must be listed separately.
		"DD_INSIDE_CI":      {},
		"DD_PROXY_NO_PROXY": {},
		"DD_PROXY_HTTP":     {},
		"DD_PROXY_HTTPS":    {},
		// these variables are used by serverless, but not via the Config struct
		"DD_API_KEY_SECRET_ARN":        {},
		"DD_DOTNET_TRACER_HOME":        {},
		"DD_SERVERLESS_APPSEC_ENABLED": {},
		"DD_SERVICE":                   {},
		"DD_VERSION":                   {},
		// this variable is used by CWS functional tests
		"DD_TESTS_RUNTIME_COMPILED": {},
		// this variable is used by the Kubernetes leader election mechanism
		"DD_POD_NAME": {},
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

func useHostEtc(config conf.Config) {
	if env.IsContainerized() && pathExists("/host/etc") {
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

// pathExists returns true if the given path exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// UnexpectedUnicodeCodepoint contains specifics about an occurrence of an unexpected unicode codepoint
type UnexpectedUnicodeCodepoint struct {
	codepoint rune
	reason    string
	position  int
}

// FindUnexpectedUnicode reports any _unexpected_ unicode codepoints
// found in the given 'input' string
// Unexpected here generally means invisible whitespace and control chars
func FindUnexpectedUnicode(input string) []UnexpectedUnicodeCodepoint {
	currentIndex := 0
	str := input
	results := make([]UnexpectedUnicodeCodepoint, 0)

	for len(str) > 0 {
		r, size := utf8.DecodeRuneInString(str)
		reason := ""
		switch {
		case r == utf8.RuneError:
			reason = "RuneError"
		case r == ' ' || r == '\r' || r == '\n' || r == '\t':
			// These are allowed whitespace
			reason = ""
		case unicode.IsSpace(r):
			reason = "unsupported whitespace"
		case unicode.Is(unicode.Bidi_Control, r):
			reason = "Bidirectional control"
		case unicode.Is(unicode.C, r):
			reason = "Control/surrogate"
		}

		if reason != "" {
			results = append(results, UnexpectedUnicodeCodepoint{
				codepoint: r,
				reason:    reason,
				position:  currentIndex,
			})
		}

		currentIndex += size
		str = str[size:]
	}
	return results
}

func setupFipsEndpoints(config conf.Config) error {
	// Each port is dedicated to a specific data type:
	//
	// port_range_start: HAProxy stats
	// port_range_start + 1:  metrics
	// port_range_start + 2:  traces
	// port_range_start + 3:  profiles
	// port_range_start + 4:  processes
	// port_range_start + 5:  logs
	// port_range_start + 6:  databases monitoring metrics
	// port_range_start + 7:  databases monitoring samples
	// port_range_start + 8:  network devices metadata
	// port_range_start + 9:  network devices snmp traps (unused)
	// port_range_start + 10: instrumentation telemetry
	// port_range_start + 11: appsec events (unused)
	// port_range_start + 12: orchestrator explorer
	// port_range_start + 13: runtime security

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
	)

	localAddress, err := conf.IsLocalAddress(config.GetString("fips.local_address"))
	if err != nil {
		return fmt.Errorf("fips.local_address: %s", err)
	}

	portRangeStart := config.GetInt("fips.port_range_start")
	urlFor := func(port int) string { return net.JoinHostPort(localAddress, strconv.Itoa(portRangeStart+port)) }

	log.Warnf("FIPS mode is enabled! All communication to DataDog will be routed to the local FIPS proxy on '%s' starting from port %d", localAddress, portRangeStart)

	// Disabling proxy to make sure all data goes directly to the FIPS proxy
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")

	// HTTP for now, will soon be updated to HTTPS
	protocol := "http://"
	if config.GetBool("fips.https") {
		protocol = "https://"
		config.Set("skip_ssl_validation", !config.GetBool("fips.tls_verify"))
	}

	// The following overwrites should be sync with the documentation for the fips.enabled config setting in the
	// config_template.yaml

	// Metrics
	config.Set("dd_url", protocol+urlFor(metrics))

	// Logs
	setupFipsLogsConfig(config, "logs_config.", urlFor(logs))

	// APM
	config.Set("apm_config.apm_dd_url", protocol+urlFor(traces))
	// Adding "/api/v2/profile" because it's not added to the 'apm_config.profiling_dd_url' value by the Agent
	config.Set("apm_config.profiling_dd_url", protocol+urlFor(profiles)+"/api/v2/profile")
	config.Set("apm_config.telemetry.dd_url", protocol+urlFor(instrumentationTelemetry))

	// Processes
	config.Set("process_config.process_dd_url", protocol+urlFor(processes))

	// Database monitoring
	config.Set("database_monitoring.metrics.dd_url", urlFor(databasesMonitoringMetrics))
	config.Set("database_monitoring.activity.dd_url", urlFor(databasesMonitoringMetrics))
	config.Set("database_monitoring.samples.dd_url", urlFor(databasesMonitoringSamples))

	// Network devices
	config.Set("network_devices.metadata.dd_url", urlFor(networkDevicesMetadata))

	// Orchestrator Explorer
	config.Set("orchestrator_explorer.orchestrator_dd_url", protocol+urlFor(orchestratorExplorer))

	// CWS
	setupFipsLogsConfig(config, "runtime_security_config.endpoints.", urlFor(runtimeSecurity))

	return nil
}

func setupFipsLogsConfig(config conf.Config, configPrefix string, url string) {
	config.Set(configPrefix+"use_http", true)
	config.Set(configPrefix+"logs_no_ssl", !config.GetBool("fips.https"))
	config.Set(configPrefix+"logs_dd_url", url)
}

func checkConflictingOptions(config conf.Config) error {
	// Verify that either use_podman_logs OR docker_path_override are set since they conflict
	if config.GetBool("logs_config.use_podman_logs") && config.IsSet("logs_config.docker_path_override") {
		log.Warnf("'use_podman_logs' is set to true and 'docker_path_override' is set, please use one or the other")
		return errors.New("'use_podman_logs' is set to true and 'docker_path_override' is set, please use one or the other")
	}

	return nil
}
