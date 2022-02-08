// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ns                   = "process_config"
	discoveryMinInterval = 10 * time.Minute
)

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// LoadAgentConfig load process-agent specific configurations based on the global Config object
func (a *AgentConfig) LoadAgentConfig(path string) error {
	loadEnvVariables()

	// Resolve any secrets
	if err := config.ResolveSecrets(config.Datadog, filepath.Base(path)); err != nil {
		return err
	}

	if config.Datadog.IsSet("hostname") {
		a.HostName = config.Datadog.GetString("hostname")
	}

	// The interval, in seconds, at which we will run each check. If you want consistent
	// behavior between real-time you may set the Container/ProcessRT intervals to 10.
	// Defaults to 10s for normal checks and 2s for others.
	a.setCheckInterval(ns, "container", ContainerCheckName)
	a.setCheckInterval(ns, "container_realtime", RTContainerCheckName)
	a.setCheckInterval(ns, "process", ProcessCheckName)
	a.setCheckInterval(ns, "process_realtime", RTProcessCheckName)
	a.setCheckInterval(ns, "connections", ConnectionsCheckName)

	// We don't need to check if the key exists since we already bound it to a default in InitConfig.
	// We use a minimum of 10 minutes for this value.
	discoveryInterval := config.Datadog.GetDuration("process_config.process_discovery.interval")
	if discoveryInterval < discoveryMinInterval {
		discoveryInterval = discoveryMinInterval
		_ = log.Warnf("Invalid interval for process discovery (<= %s) using default value of %[1]s", discoveryMinInterval.String())
	}
	a.CheckIntervals[DiscoveryCheckName] = discoveryInterval

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
