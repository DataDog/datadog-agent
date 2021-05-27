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
	ns = "process_config"
)

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
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

	// The maximum number of processes belonging to a container per message. Note: Only change if the defaults are causing issues.
	if k := key(ns, "max_ctr_procs_per_message"); config.Datadog.IsSet(k) {
		if maxCtrProcessesPerMessage := config.Datadog.GetInt(k); maxCtrProcessesPerMessage <= 0 {
			log.Warnf("Invalid max container processes count per message (<= 0), using default value of %d", defaultMaxCtrProcsMessageBatch)
		} else if maxCtrProcessesPerMessage <= maxCtrProcsMessageBatch {
			a.MaxCtrProcessesPerMessage = maxCtrProcessesPerMessage
		} else {
			log.Warnf("Overriding the configured max container processes count per message limit because it exceeds maximum limit of %d", maxCtrProcsMessageBatch)
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

	// use `internal_profiling.enabled` field in `process_config` section to enable/disable profiling for process-agent,
	// but use the configuration from main agent to fill the settings
	if config.Datadog.IsSet(key(ns, "internal_profiling.enabled")) {
		a.ProfilingEnabled = config.Datadog.GetBool(key(ns, "internal_profiling.enabled"))
		a.ProfilingSite = config.Datadog.GetString("site")
		a.ProfilingURL = config.Datadog.GetString("internal_profiling.profile_dd_url")
		a.ProfilingAPIKey = config.SanitizeAPIKey(config.Datadog.GetString("api_key"))
		a.ProfilingEnvironment = config.Datadog.GetString("env")
		a.ProfilingPeriod = config.Datadog.GetDuration("internal_profiling.period")
		a.ProfilingCPUDuration = config.Datadog.GetDuration("internal_profiling.cpu_duration")
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
