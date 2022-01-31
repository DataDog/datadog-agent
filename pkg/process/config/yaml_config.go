// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"net/url"
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
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	ns                   = "process_config"
	discoveryMinInterval = 10 * time.Minute
)

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// LoadProcessYamlConfig load Process-specific configuration
func (a *AgentConfig) LoadProcessYamlConfig(path string, canAccessContainers bool) error {
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

	if config.Datadog.GetBool("process_config.process_collection.enabled") {
		a.EnabledChecks = append(a.EnabledChecks, processChecks...)
	} else if config.Datadog.GetBool("process_config.container_collection.enabled") && canAccessContainers {
		// Container checks are enabled only when process checks are not (since they automatically collect container data).
		a.EnabledChecks = append(a.EnabledChecks, containerChecks...)
	}
	// The interval, in seconds, at which we will run each check. If you want consistent
	// behavior between real-time you may set the Container/ProcessRT intervals to 10.
	// Defaults to 10s for normal checks and 2s for others.
	a.setCheckInterval(ns, "container", ContainerCheckName)
	a.setCheckInterval(ns, "container_realtime", RTContainerCheckName)
	a.setCheckInterval(ns, "process", ProcessCheckName)
	a.setCheckInterval(ns, "process_realtime", RTProcessCheckName)
	a.setCheckInterval(ns, "connections", ConnectionsCheckName)

	// We need another method to read in process discovery check configs because it is in its own object,
	// and uses a different unit of time
	a.initProcessDiscoveryCheck()

	if a.CheckIntervals[ProcessCheckName] < a.CheckIntervals[RTProcessCheckName] || a.CheckIntervals[ProcessCheckName]%a.CheckIntervals[RTProcessCheckName] != 0 {
		// Process check interval must be greater or equal to RTProcess check interval and the intervals must be divisible
		// in order to be run on the same goroutine
		log.Warnf(
			"Invalid process check interval overrides [%s,%s], resetting to defaults [%s,%s]",
			a.CheckIntervals[ProcessCheckName],
			a.CheckIntervals[RTProcessCheckName],
			ProcessCheckDefaultInterval,
			RTProcessCheckDefaultInterval,
		)
		a.CheckIntervals[ProcessCheckName] = ProcessCheckDefaultInterval
		a.CheckIntervals[RTProcessCheckName] = RTProcessCheckDefaultInterval
	}

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

	// Windows: Sets windows process table refresh rate (in number of check runs)
	if argRefresh := config.Datadog.GetInt(key(ns, "windows", "args_refresh_interval")); argRefresh != 0 {
		a.Windows.ArgsRefreshInterval = argRefresh
	}

	// Windows: Controls getting process arguments immediately when a new process is discovered
	if addArgsKey := key(ns, "windows", "add_new_args"); config.Datadog.IsSet(addArgsKey) {
		a.Windows.AddNewArgs = config.Datadog.GetBool(addArgsKey)
	}

	// Windows: Controls using the new check based on performance counters PDH APIs
	if usePerfCountersKey := key(ns, "windows", "use_perf_counters"); config.Datadog.IsSet(usePerfCountersKey) {
		a.Windows.UsePerfCounters = config.Datadog.GetBool(usePerfCountersKey)
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
	if !config.Datadog.IsSet(key(ns, "cmd_port")) {
		config.Datadog.Set(key(ns, "cmd_port"), 6162)
	}

	// use `internal_profiling.enabled` field in `process_config` section to enable/disable profiling for process-agent,
	// but use the configuration from main agent to fill the settings
	if config.Datadog.IsSet(key(ns, "internal_profiling.enabled")) {
		// allow full url override for development use
		site := config.Datadog.GetString("internal_profiling.profile_dd_url")
		if site == "" {
			s := config.Datadog.GetString("site")
			if s == "" {
				s = config.DefaultSite
			}
			site = fmt.Sprintf(profiling.ProfilingURLTemplate, s)
		}

		v, _ := version.Agent()
		a.ProfilingSettings = &profiling.Settings{
			ProfilingURL:         site,
			Env:                  config.Datadog.GetString("env"),
			Service:              "process-agent",
			Period:               config.Datadog.GetDuration("internal_profiling.period"),
			CPUDuration:          config.Datadog.GetDuration("internal_profiling.cpu_duration"),
			MutexProfileFraction: config.Datadog.GetInt("internal_profiling.mutex_profile_fraction"),
			BlockProfileRate:     config.Datadog.GetInt("internal_profiling.block_profile_rate"),
			WithGoroutineProfile: config.Datadog.GetBool("internal_profiling.enable_goroutine_stacktraces"),
			Tags:                 []string{fmt.Sprintf("version:%v", v)},
		}
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
		log.Infof("Overriding %s check interval to %ds", checkKey, interval)
		a.CheckIntervals[checkKey] = time.Duration(interval) * time.Second
	}
}

// Separate handler for initializing the process discovery check.
// Since it has its own unique object, we need to handle loading in the check config differently separately
// from the other checks.
func (a *AgentConfig) initProcessDiscoveryCheck() {
	if config.IsECSFargate() {
		log.Debug("Process discovery is not supported on ECS Fargate")
		return
	}

	root := key(ns, "process_discovery")

	// Discovery check can only be enabled when regular process collection is not enabled.
	processCheckEnabled := config.Datadog.GetBool("process_config.process_collection.enabled")
	discoveryCheckEnabled := config.Datadog.GetBool(key(root, "enabled"))
	if discoveryCheckEnabled && !processCheckEnabled {
		a.EnabledChecks = append(a.EnabledChecks, DiscoveryCheckName)

		// We don't need to check if the key exists since we already bound it to a default in InitConfig.
		// We use a minimum of 10 minutes for this value.
		discoveryInterval := config.Datadog.GetDuration(key(root, "interval"))
		if discoveryInterval < discoveryMinInterval {
			discoveryInterval = discoveryMinInterval
			_ = log.Warnf("Invalid interval for process discovery (<= %s) using default value of %[1]s", discoveryMinInterval.String())
		}
		a.CheckIntervals[DiscoveryCheckName] = discoveryInterval
	}
}
