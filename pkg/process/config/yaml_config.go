package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	// Resolve any secrets
	if err := config.ResolveSecrets(config.Datadog, filepath.Base(path)); err != nil {
		return err
	}

	// Whether agent should disable collection for TCP, UDP, or IPv6 connection type respectively
	a.DisableTCPTracing = config.Datadog.GetBool(key(spNS, "disable_tcp"))
	a.DisableUDPTracing = config.Datadog.GetBool(key(spNS, "disable_udp"))
	a.DisableIPv6Tracing = config.Datadog.GetBool(key(spNS, "disable_ipv6"))
	if config.Datadog.IsSet(key(spNS, "disable_dns_inspection")) {
		a.DisableDNSInspection = config.Datadog.GetBool(key(spNS, "disable_dns_inspection"))
	}

	a.CollectLocalDNS = config.Datadog.GetBool(key(spNS, "collect_local_dns"))

	if config.Datadog.IsSet(key(spNS, "collect_dns_stats")) {
		a.CollectDNSStats = config.Datadog.GetBool(key(spNS, "collect_dns_stats"))
	}

	if config.Datadog.IsSet(key(spNS, "max_dns_stats")) {
		a.MaxDNSStats = config.Datadog.GetInt(key(spNS, "max_dns_stats"))
	}

	if config.Datadog.IsSet(key(spNS, "collect_dns_domains")) {
		a.CollectDNSDomains = config.Datadog.GetBool(key(spNS, "collect_dns_domains"))
	}

	if config.Datadog.IsSet(key(spNS, "dns_timeout_in_s")) {
		a.DNSTimeout = config.Datadog.GetDuration(key(spNS, "dns_timeout_in_s")) * time.Second
	}

	if config.Datadog.IsSet("network_config.enable_http_monitoring") {
		a.EnableHTTPMonitoring = config.Datadog.GetBool("network_config.enable_http_monitoring")
	}

	if config.Datadog.IsSet("network_config.ignore_conntrack_init_failure") {
		a.IgnoreConntrackInitFailure = config.Datadog.GetBool("network_config.ignore_conntrack_init_failure")
	}

	if config.Datadog.GetBool(key(spNS, "enabled")) {
		a.EnableSystemProbe = true
	}

	a.SysProbeBPFDebug = config.Datadog.GetBool(key(spNS, "bpf_debug"))
	if config.Datadog.IsSet(key(spNS, "bpf_dir")) {
		a.SystemProbeBPFDir = config.Datadog.GetString(key(spNS, "bpf_dir"))
	}

	if config.Datadog.IsSet(key(spNS, "excluded_linux_versions")) {
		a.ExcludedBPFLinuxVersions = config.Datadog.GetStringSlice(key(spNS, "excluded_linux_versions"))
	}

	// The full path to the location of the unix socket where connections will be accessed
	if socketPath := config.Datadog.GetString(key(spNS, "sysprobe_socket")); socketPath != "" {
		if err := ValidateSysprobeSocket(socketPath); err != nil {
			log.Errorf("Could not parse %s.sysprobe_socket: %s", spNS, err)
		} else {
			a.SystemProbeAddress = socketPath
		}
	}

	if config.Datadog.IsSet(key(spNS, "enable_conntrack")) {
		a.EnableConntrack = config.Datadog.GetBool(key(spNS, "enable_conntrack"))
	}
	if s := config.Datadog.GetInt(key(spNS, "conntrack_max_state_size")); s > 0 {
		a.ConntrackMaxStateSize = s
	}
	if config.Datadog.IsSet(key(spNS, "conntrack_rate_limit")) {
		a.ConntrackRateLimit = config.Datadog.GetInt(key(spNS, "conntrack_rate_limit"))
	}
	if config.Datadog.IsSet(key(spNS, "enable_conntrack_all_namespaces")) {
		a.EnableConntrackAllNamespaces = config.Datadog.GetBool(key(spNS, "enable_conntrack_all_namespaces"))
	}

	// When reading kernel structs at different offsets, don't go over the threshold
	// This defaults to 400 and has a max of 3000. These are arbitrary choices to avoid infinite loops.
	if th := config.Datadog.GetInt(key(spNS, "offset_guess_threshold")); th > 0 {
		if th < maxOffsetThreshold {
			a.OffsetGuessThreshold = uint64(th)
		} else {
			log.Warn("offset_guess_threshold exceeds maximum of 3000. Setting it to the default of 400")
		}
	}

	if logFile := config.Datadog.GetString(key(spNS, "log_file")); logFile != "" {
		a.SystemProbeLogFile = logFile
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
		a.MaxTrackedConnections = uint(mtc)
	}

	// MaxClosedConnectionsBuffered represents the maximum number of closed connections we'll buffer in memory. These closed connections
	// get flushed on every client request (default 30s check interval)
	if k := key(spNS, "max_closed_connections_buffered"); config.Datadog.IsSet(k) {
		if mcb := config.Datadog.GetInt(k); mcb > 0 {
			a.MaxClosedConnectionsBuffered = mcb
		}
	}

	// MaxConnectionsStateBuffered represents the maximum number of state objects that we'll store in memory. These state objects store
	// the stats for a connection so we can accurately determine traffic change between client requests.
	if k := key(spNS, "max_connection_state_buffered"); config.Datadog.IsSet(k) {
		if mcsb := config.Datadog.GetInt(k); mcsb > 0 {
			a.MaxConnectionsStateBuffered = mcsb
		}
	}

	if ccs := config.Datadog.GetInt(key(spNS, "closed_channel_size")); ccs > 0 {
		a.ClosedChannelSize = ccs
	}

	// Pull additional parameters from the global config file.
	a.LogLevel = config.Datadog.GetString("log_level")
	a.StatsdPort = config.Datadog.GetInt("dogstatsd_port")

	// The tcp port that agent should expose expvar and pprof endpoint to
	if debugPort := config.Datadog.GetInt(key(spNS, "debug_port")); debugPort > 0 {
		a.SystemProbeDebugPort = debugPort
	}

	if sourceExclude := key(spNS, "source_excludes"); config.Datadog.IsSet(sourceExclude) {
		a.ExcludedSourceConnections = config.Datadog.GetStringMapStringSlice(sourceExclude)
	}

	if destinationExclude := key(spNS, "dest_excludes"); config.Datadog.IsSet(destinationExclude) {
		a.ExcludedDestinationConnections = config.Datadog.GetStringMapStringSlice(destinationExclude)
	}

	if config.Datadog.GetBool(key(spNS, "enable_tcp_queue_length")) {
		log.Info("system_probe_config.enable_tcp_queue_length detected, will enable system-probe with TCP queue length check")
		a.EnableSystemProbe = true
		a.EnabledChecks = append(a.EnabledChecks, TCPQueueLengthCheckName)
	}

	if config.Datadog.GetBool(key(spNS, "process_config.enabled")) {
		a.EnableSystemProbe = true
		a.EnabledChecks = append(a.EnabledChecks, ProcessModuleCheckName)
	}

	if config.Datadog.GetBool(key(spNS, "enable_oom_kill")) {
		log.Info("system_probe_config.enable_oom_kill detected, will enable system-probe with OOM Kill check")
		a.EnableSystemProbe = true
		a.EnabledChecks = append(a.EnabledChecks, OOMKillCheckName)
	}

	if config.Datadog.GetBool("runtime_security_config.enabled") || config.Datadog.GetBool("runtime_security_config.fim_enabled") {
		log.Info("runtime_security_config.enabled or runtime_security_config.fim_enabled detected, enabling system-probe")
		a.EnableSystemProbe = true
	}

	if config.Datadog.IsSet(key(spNS, "enable_tracepoints")) {
		a.EnableTracepoints = config.Datadog.GetBool(key(spNS, "enable_tracepoints"))
	}

	a.Windows.EnableMonotonicCount = config.Datadog.GetBool(key(spNS, "windows", "enable_monotonic_count"))

	if driverBufferSize := config.Datadog.GetInt(key(spNS, "windows", "driver_buffer_size")); driverBufferSize > 0 {
		a.Windows.DriverBufferSize = driverBufferSize
	}

	// Enable network and connections check
	if config.Datadog.GetBool("network_config.enabled") {
		log.Info(fmt.Sprintf("network_config.enabled detected: enabling system-probe with network module running."))
		a.EnabledChecks = append(a.EnabledChecks, ConnectionsCheckName, NetworkCheckName)
		a.EnableSystemProbe = true // system-probe is implicitly enabled if networks is enabled
	} else if config.Datadog.IsSet(key(spNS, "enabled")) && config.Datadog.GetBool(key(spNS, "enabled")) && !config.Datadog.IsSet(key("network_config", "enabled")) {
		// This case exists to preserve backwards compatibility. If system_probe_config.enabled is explicitly set to true, and there is no network_config block,
		// enable the connections/network check.
		log.Info("network_config not found, but system-probe was enabled, enabling network module by default")
		a.EnabledChecks = append(a.EnabledChecks, NetworkCheckName, ConnectionsCheckName)
		a.EnableSystemProbe = true
	}

	if !a.Enabled && util.StringInSlice(a.EnabledChecks, ConnectionsCheckName) {
		log.Info("enabling process-agent for connections check as the system-probe is enabled")
		a.Enabled = true
	}

	if config.Datadog.IsSet(key(spNS, "profiling.enabled")) {
		a.ProfilingEnabled = config.Datadog.GetBool(key(spNS, "profiling.enabled"))
		a.ProfilingSite = config.Datadog.GetString(key(spNS, "profiling.site"))
		a.ProfilingURL = config.Datadog.GetString(key(spNS, "profiling.profile_dd_url"))
		a.ProfilingAPIKey = config.SanitizeAPIKey(config.Datadog.GetString(key(spNS, "profiling.api_key")))
		a.ProfilingEnvironment = config.Datadog.GetString(key(spNS, "profiling.env"))
	}
	a.EnableRuntimeCompiler = config.Datadog.GetBool(key(spNS, "enable_runtime_compiler"))
	if config.Datadog.IsSet(key(spNS, "kernel_header_dirs")) {
		a.KernelHeadersDirs = config.Datadog.GetStringSlice(key(spNS, "kernel_header_dirs"))
	}

	if config.Datadog.IsSet(key(spNS, "runtime_compiler_output_dir")) {
		a.RuntimeCompilerOutputDir = config.Datadog.GetString(key(spNS, "runtime_compiler_output_dir"))
	}

	if config.Datadog.IsSet("network_config.enable_gateway_lookup") {
		a.EnableGatewayLookup = config.Datadog.GetBool("network_config.enable_gateway_lookup")
	}

	return nil
}

// LoadProcessYamlConfig load Process-specific configuration
func (a *AgentConfig) LoadProcessYamlConfig(path string) error {
	loadEnvVariables()

	// Resolve any secrets
	if err := config.ResolveSecrets(config.Datadog, filepath.Base(path)); err != nil {
		return err
	}

	URL, err := url.Parse(config.GetMainEndpoint("https://process.", key(ns, "process_dd_url")))
	if err != nil {
		return fmt.Errorf("error parsing process_dd_url: %s", err)
	}
	a.APIEndpoints[0].Endpoint = URL

	if key := "api_key"; config.Datadog.IsSet(key) {
		a.APIEndpoints[0].APIKey = config.SanitizeAPIKey(config.Datadog.GetString(key))
	}

	if config.Datadog.IsSet("hostname") {
		a.HostName = config.Datadog.GetString("hostname")
	}

	// Note: The enabled environment flag operates differently than that of our YAML configuration
	if v, ok := os.LookupEnv("DD_PROCESS_AGENT_ENABLED"); ok {
		// DD_PROCESS_AGENT_ENABLED: true - Process + Container checks enabled
		//                           false - No checks enabled
		//                           (none) - Container check enabled (by default)
		if enabled, err := isAffirmative(v); enabled {
			a.Enabled = true
			a.EnabledChecks = processChecks
		} else if !enabled && err == nil {
			a.Enabled = false
		}
	} else if k := key(ns, "enabled"); config.Datadog.IsSet(k) {
		// A string indicate the enabled state of the Agent.
		//   If "false" (the default) we will only collect containers.
		//   If "true" we will collect containers and processes.
		//   If "disabled" the agent will be disabled altogether and won't start.
		enabled := config.Datadog.GetString(k)
		ok, err := isAffirmative(enabled)
		if ok {
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
	a.setCheckInterval(ns, "container", ContainerCheckName)
	a.setCheckInterval(ns, "container_realtime", RTContainerCheckName)
	a.setCheckInterval(ns, "process", ProcessCheckName)
	a.setCheckInterval(ns, "process_realtime", RTProcessCheckName)
	a.setCheckInterval(ns, "connections", ConnectionsCheckName)

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

	if k := key(ns, "process_queue_bytes"); config.Datadog.IsSet(k) {
		if queueBytes := config.Datadog.GetInt(k); queueBytes > 0 {
			a.ProcessQueueBytes = queueBytes
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

	// Overrides the grpc connection timeout setting to the main agent.
	if k := key(ns, "grpc_connection_timeout_secs"); config.Datadog.IsSet(k) {
		a.grpcConnectionTimeout = config.Datadog.GetDuration(k) * time.Second
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
				a.APIEndpoints = append(a.APIEndpoints, apicfg.Endpoint{
					APIKey:   config.SanitizeAPIKey(k),
					Endpoint: u,
				})
			}
		}
	}

	// use `profiling.enabled` field in `process_config` section to enable/disable profiling for process-agent,
	// but use the configuration from main agent to fill the settings
	if config.Datadog.IsSet(key(ns, "profiling.enabled")) {
		a.ProfilingEnabled = config.Datadog.GetBool(key(ns, "profiling.enabled"))
		a.ProfilingSite = config.Datadog.GetString("site")
		a.ProfilingURL = config.Datadog.GetString("profiling.profile_dd_url")
		a.ProfilingAPIKey = config.SanitizeAPIKey(config.Datadog.GetString("api_key"))
		a.ProfilingEnvironment = config.Datadog.GetString("env")
	}

	// Used to override container source auto-detection
	// and to enable multiple collector sources if needed.
	// "docker", "ecs_fargate", "kubelet", "kubelet docker", etc.
	containerSourceKey := key(ns, "container_source")
	if config.Datadog.Get(containerSourceKey) != nil {
		// container_source can be nil since we're not forcing default values in the main config file
		// make sure we don't pass nil value to GetStringSlice to avoid spammy warnings
		if sources := config.Datadog.GetStringSlice(containerSourceKey); len(sources) > 0 {
			util.SetContainerSources(sources)
		}
	}

	// Pull additional parameters from the global config file.
	if level := config.Datadog.GetString("log_level"); level != "" {
		a.LogLevel = level
	}

	if k := "dogstatsd_port"; config.Datadog.IsSet(k) {
		a.StatsdPort = config.Datadog.GetInt(k)
	}

	if bindHost := config.GetBindHost(); bindHost != "" {
		a.StatsdHost = bindHost
	}

	// Build transport (w/ proxy if needed)
	a.Transport = httputils.CreateHTTPTransport()

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
