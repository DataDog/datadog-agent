// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"fmt"
	"runtime"
	"slices"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type logsProfileSetting struct {
	key   string
	value any
}

// logsPerformanceProfile is a pre-tuned bundle of logs_config.* settings.
type logsPerformanceProfile struct {
	name        string
	description string
	settings    []logsProfileSetting
}

// pipelinesUpToCores resolves to min(n, GOMAXPROCS) at apply time.
type pipelinesUpToCores int

func resolveProfileValue(v any) any {
	if n, ok := v.(pipelinesUpToCores); ok {
		return min(int(n), runtime.GOMAXPROCS(0))
	}
	return v
}

// Throughput profiles are supersets: high-concurrency ⊂ high-throughput ⊂ max-throughput.
var logsPerformanceProfiles = []logsPerformanceProfile{
	{
		name:        "high-concurrency",
		description: "Maximize concurrent in-flight sends to absorb intake/network latency (send/transport bottleneck).",
		settings: []logsProfileSetting{
			{"logs_config.batch_max_concurrent_send", 20},
			{"logs_config.payload_channel_size", 40},
		},
	},
	{
		name:        "high-throughput",
		description: "Maximize sustained log throughput (one pipeline per core up to 16, high send concurrency, larger buffers).",
		settings: []logsProfileSetting{
			{"logs_config.batch_max_concurrent_send", 20},
			{"logs_config.payload_channel_size", 40},
			{"logs_config.pipelines", pipelinesUpToCores(16)},
			{"logs_config.message_channel_size", 200},
		},
	},
	{
		name:        "max-throughput",
		description: "Remove the compression CPU bottleneck (high-throughput plus compression disabled, including for HTTP additional_endpoints), at the expense of bandwidth and memory.",
		settings: []logsProfileSetting{
			{"logs_config.batch_max_concurrent_send", 20},
			{"logs_config.payload_channel_size", 40},
			{"logs_config.pipelines", pipelinesUpToCores(16)},
			{"logs_config.message_channel_size", 200},
			{"logs_config.use_compression", false},
		},
	},
	{
		name:        "low-latency",
		description: "Minimize log delivery latency (short batch wait).",
		settings: []logsProfileSetting{
			{"logs_config.batch_wait", 1.0},
		},
	},
}

func findLogsPerformanceProfile(name string) (logsPerformanceProfile, bool) {
	for _, p := range logsPerformanceProfiles {
		if p.name == name {
			return p, true
		}
	}
	return logsPerformanceProfile{}, false
}

func (p logsPerformanceProfile) value(key string) (any, bool) {
	for _, s := range p.settings {
		if s.key == key {
			return s.value, true
		}
	}
	return nil, false
}

// logsProfileKeys returns every key any profile can write.
func logsProfileKeys() []string {
	var keys []string
	for _, p := range logsPerformanceProfiles {
		for _, s := range p.settings {
			if !slices.Contains(keys, s.key) {
				keys = append(keys, s.key)
			}
		}
	}
	return keys
}

// profileYieldsTo reports whether a value from source takes precedence over the profile.
func profileYieldsTo(source pkgconfigmodel.Source) bool {
	switch source {
	case pkgconfigmodel.SourceFile,
		pkgconfigmodel.SourceEnvVar,
		pkgconfigmodel.SourceFleetPolicies,
		pkgconfigmodel.SourceSecret,
		pkgconfigmodel.SourceLocalConfigProcess,
		pkgconfigmodel.SourceAgentRuntime,
		pkgconfigmodel.SourceRC,
		pkgconfigmodel.SourceCLI:
		return true
	}
	return false
}

type profileKeyState struct {
	explicit bool
	// setAtLoad is an explicit value from a source other than fleet policies, which merge between override passes.
	// The profile never writes such a key, so a post-init value on it is a schema-enforcement revert.
	setAtLoad bool
	postInit  any
}

func readProfileKeyState(config pkgconfigmodel.Reader, key string) profileKeyState {
	var st profileKeyState
	for _, vs := range config.GetAllSources(key) {
		if vs.Value == nil {
			continue
		}
		switch {
		case vs.Source == pkgconfigmodel.SourceConfigPostInit:
			st.postInit = vs.Value
		case profileYieldsTo(vs.Source):
			st.explicit = true
			st.setAtLoad = st.setAtLoad || vs.Source != pkgconfigmodel.SourceFleetPolicies
		}
	}
	return st
}

// ApplyLogsPerformanceProfile expands logs_config.profile at SourceConfigPostInit; it is idempotent.
func ApplyLogsPerformanceProfile(config pkgconfigmodel.Config) {
	name := config.GetString("logs_config.profile")
	profile, found := findLogsPerformanceProfile(name)
	if !found && name != "" {
		names := make([]string, 0, len(logsPerformanceProfiles))
		for _, p := range logsPerformanceProfiles {
			names = append(names, p.name)
		}
		log.Errorf("Unknown logs_config.profile %q; ignoring and using default logs settings. Available profiles: %s",
			name, strings.Join(names, ", "))
	}

	var kept []string
	for _, key := range logsProfileKeys() {
		value, inProfile := profile.value(key)
		st := readProfileKeyState(config, key)
		switch {
		case inProfile && !st.explicit:
			if resolved := resolveProfileValue(value); st.postInit != resolved {
				config.Set(key, resolved, pkgconfigmodel.SourceConfigPostInit)
			}
		case st.postInit != nil && !st.setAtLoad:
			config.UnsetForSource(key, pkgconfigmodel.SourceConfigPostInit)
		}
		if inProfile && st.explicit {
			kept = append(kept, fmt.Sprintf("%s (%v)", key, config.Get(key)))
		}
	}

	if !found {
		return
	}
	log.Infof("Applied logs performance profile %q: %s", name, profile.description)
	if len(kept) > 0 {
		log.Infof("Logs performance profile %q kept explicitly-configured settings: %s", name, strings.Join(kept, ", "))
	}
}
