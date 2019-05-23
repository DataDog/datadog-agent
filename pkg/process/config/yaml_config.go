package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/config"
	ddutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const (
	ns   = "process_config"
	spNS = "system_probe_config"
)

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// SystemProbe specific configuration
func (a *AgentConfig) loadSysProbeYamlConfig(path string) error {
	loadEnvVariables()

	a.EnableLocalSystemProbe = config.Datadog.GetBool(key(spNS, "use_local_system_probe"))

	// Whether agent should disable collection for TCP, UDP, or IPv6 connection type respectively
	a.DisableTCPTracing = config.Datadog.GetBool(key(spNS, "disable_tcp"))
	a.DisableUDPTracing = config.Datadog.GetBool(key(spNS, "disable_udp"))
	a.DisableIPv6Tracing = config.Datadog.GetBool(key(spNS, "disable_ipv6"))

	a.CollectLocalDNS = config.Datadog.GetBool(key(spNS, "collect_local_dns"))

	// Whether agent should expose profiling endpoints over the unix socket
	a.EnableDebugProfiling = config.Datadog.GetBool(key(spNS, "debug_profiling_enabled"))

	if config.Datadog.GetBool(key(spNS, "enabled")) {
		a.EnabledChecks = append(a.EnabledChecks, "connections")
		a.EnableSystemProbe = true
	}

	a.SysProbeBPFDebug = config.Datadog.GetBool(key(spNS, "bpf_debug"))
	if config.Datadog.IsSet(key(spNS, "excluded_linux_versions")) {
		a.ExcludedBPFLinuxVersions = config.Datadog.GetStringSlice(key(spNS, "excluded_linux_versions"))
	}

	// The full path to the location of the unix socket where connections will be accessed
	if socketPath := config.Datadog.GetString(key(spNS, "sysprobe_socket")); socketPath != "" {
		a.SystemProbeSocketPath = socketPath
	}

	if config.Datadog.IsSet(key(spNS, "enable_conntrack")) {
		a.EnableConntrack = config.Datadog.GetBool(key(spNS, "enable_conntrack"))
	}
	if s := config.Datadog.GetInt(key(spNS, "conntrack_short_term_buffer_size")); s > 0 {
		a.ConntrackShortTermBufferSize = s
	}

	if logFile := config.Datadog.GetString(key(spNS, "log_file")); logFile != "" {
		a.LogFile = logFile
	}

	// The maximum number of connections per message. Note: Only change if the defaults are causing issues.
	if mcpm := config.Datadog.GetInt(key(spNS, "max_conns_per_message")); mcpm > 0 {
		if mcpm <= maxConnsMessageBatch {
			a.MaxConnsPerMessage = mcpm
		} else {
			log.Warn("Overriding the configured connections count per message limit because it exceeds maximum")
		}
	}

	// The maximum number of connections the tracer can track
	if mtc := config.Datadog.GetInt64(key(spNS, "max_tracked_connections")); mtc > 0 {
		if mtc <= maxMaxTrackedConnections {
			a.MaxTrackedConnections = uint(mtc)
		} else {
			log.Warnf("Overriding the configured max tracked connections limit because it exceeds maximum 65536, got: %v", mtc)
		}
	}

	// Pull additional parameters from the global config file.
	a.LogLevel = config.Datadog.GetString("log_level")
	a.StatsdPort = config.Datadog.GetInt("dogstatsd_port")

	return nil
}

// Process-specific configuration
func (a *AgentConfig) loadProcessYamlConfig(path string) error {
	loadEnvVariables()

	URL, err := url.Parse(config.GetMainEndpoint("https://process.", key(ns, "process_dd_url")))
	if err != nil {
		return fmt.Errorf("error parsing process_dd_url: %s", err)
	}

	a.APIEndpoints[0].Endpoint = URL
	if key := "api_key"; config.Datadog.IsSet(key) {
		a.APIEndpoints[0].APIKey = config.Datadog.GetString(key)
	}

	if k := key(ns, "enabled"); config.Datadog.IsSet(k) {
		// A string indicate the enabled state of the Agent.
		// If "false" (the default) we will only collect containers.
		// If "true" we will collect containers and processes.
		// If "disabled" the agent will be disabled altogether and won't start.
		enabled := config.Datadog.GetString(k)
		if ok, err := isAffirmative(enabled); ok {
			a.Enabled, a.EnabledChecks = true, processChecks
		} else if enabled == "disabled" {
			a.Enabled = false
		} else if !ok && err == nil {
			a.Enabled, a.EnabledChecks = true, containerChecks
		}
	}

	// Whether or not the process-agent should output logs to console
	if config.Datadog.GetBool("log_to_console") {
		a.LogToConsole = true
	}
	// The full path to the file where process-agent logs will be written.
	if logFile := config.Datadog.GetString(key(ns, "log_file")); logFile != "" {
		a.LogFile = logFile
	}

	// The interval, in seconds, at which we will run each check. If you want consistent
	// behavior between real-time you may set the Container/ProcessRT intervals to 10.
	// Defaults to 10s for normal checks and 2s for others.
	a.setCheckInterval(ns, "container", "container")
	a.setCheckInterval(ns, "container_realtime", "rtcontainer")
	a.setCheckInterval(ns, "process", "process")
	a.setCheckInterval(ns, "process_realtime", "rtprocess")
	a.setCheckInterval(ns, "connections", "connections")

	// A list of regex patterns that will exclude a process if matched.
	if k := key(ns, "blacklist_patterns"); config.Datadog.IsSet(k) {
		for _, b := range config.Datadog.GetStringSlice(k) {
			r, err := regexp.Compile(b)
			if err != nil {
				log.Warnf("Ignoring invalid blacklist pattern: %s", b)
				continue
			}
			a.Blacklist = append(a.Blacklist, r)
		}
	}

	if k := key(ns, "expvar_port"); config.Datadog.IsSet(k) {
		port := config.Datadog.GetInt(k)
		if port <= 0 {
			return errors.Errorf("invalid %s -- %d", k, port)
		}
		a.ProcessExpVarPort = port
	}

	// Enable/Disable the DataScrubber to obfuscate process args
	if scrubArgsKey := key(ns, "scrub_args"); config.Datadog.IsSet(scrubArgsKey) {
		a.Scrubber.Enabled = config.Datadog.GetBool(scrubArgsKey)
	}

	// A custom word list to enhance the default one used by the DataScrubber
	if k := key(ns, "custom_sensitive_words"); config.Datadog.IsSet(k) {
		a.Scrubber.AddCustomSensitiveWords(config.Datadog.GetStringSlice(k))
	}

	// Strips all process arguments
	if config.Datadog.GetBool(key(ns, "strip_proc_arguments")) {
		a.Scrubber.StripAllArguments = true
	}

	// How many check results to buffer in memory when POST fails. The default is usually fine.
	if k := key(ns, "queue_size"); config.Datadog.IsSet(k) {
		if queueSize := config.Datadog.GetInt(k); queueSize > 0 {
			a.QueueSize = queueSize
		}
	}

	// The maximum number of processes, or containers per message. Note: Only change if the defaults are causing issues.
	if k := key(ns, "max_per_message"); config.Datadog.IsSet(k) {
		if maxPerMessage := config.Datadog.GetInt(k); maxPerMessage <= 0 {
			log.Warn("Invalid item count per message (<= 0), ignoring...")
		} else if maxPerMessage <= maxMessageBatch {
			a.MaxPerMessage = maxPerMessage
		} else if maxPerMessage > 0 {
			log.Warn("Overriding the configured item count per message limit because it exceeds maximum")
		}
	}

	// Overrides the path to the Agent bin used for getting the hostname. The default is usually fine.
	a.DDAgentBin = defaultDDAgentBin
	if k := key(ns, "dd_agent_bin"); config.Datadog.IsSet(k) {
		if agentBin := config.Datadog.GetString(k); agentBin != "" {
			a.DDAgentBin = agentBin
		}
	}

	// Windows: Sets windows process table refresh rate (in number of check runs)
	if argRefresh := config.Datadog.GetInt(key(ns, "windows", "args_refresh_interval")); argRefresh != 0 {
		a.Windows.ArgsRefreshInterval = argRefresh
	}

	// Windows: Controls getting process arguments immediately when a new process is discovered
	if addArgsKey := key(ns, "windows", "add_new_args"); config.Datadog.IsSet(addArgsKey) {
		a.Windows.AddNewArgs = config.Datadog.GetBool(addArgsKey)
	}

	// Optional additional pairs of endpoint_url => []apiKeys to submit to other locations.
	if k := key(ns, "additional_endpoints"); config.Datadog.IsSet(k) {
		for endpointURL, apiKeys := range config.Datadog.GetStringMapStringSlice(k) {
			u, err := URL.Parse(endpointURL)
			if err != nil {
				return fmt.Errorf("invalid additional endpoint url '%s': %s", endpointURL, err)
			}
			for _, k := range apiKeys {
				a.APIEndpoints = append(a.APIEndpoints, APIEndpoint{
					APIKey:   k,
					Endpoint: u,
				})
			}
		}
	}

	// Used to override container source auto-detection.
	// "docker", "ecs_fargate", "kubelet", etc
	if containerSource := config.Datadog.GetString(key(ns, "container_source")); containerSource != "" {
		util.SetContainerSource(containerSource)
	}

	// Pull additional parameters from the global config file.
	if level := config.Datadog.GetString("log_level"); level != "" {
		a.LogLevel = level
	}

	if k := "dogstatsd_port"; config.Datadog.IsSet(k) {
		a.StatsdPort = config.Datadog.GetInt(k)
	}

	if bindHost := config.Datadog.GetString(key(ns, "bind_host")); bindHost != "" {
		a.StatsdHost = bindHost
	}

	// Build transport (w/ proxy if needed)
	a.Transport = ddutil.CreateHTTPTransport()

	return nil
}

func (a *AgentConfig) setCheckInterval(ns, check, checkKey string) {
	k := key(ns, "intervals", check)

	if !config.Datadog.IsSet(k) {
		return
	}

	if interval := config.Datadog.GetInt(k); interval != 0 {
		log.Infof("Overriding container check interval to %ds", interval)
		a.CheckIntervals[checkKey] = time.Duration(interval) * time.Second
	}
}
