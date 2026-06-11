// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"fmt"
	"sort"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// logsPerformanceProfile is a curated, pre-tuned bundle of logs_config.*
// performance settings. A profile lets users opt into a known-good
// configuration for a given workload shape without having to understand and
// tune each individual logs_config.* knob.
type logsPerformanceProfile struct {
	// description is a short human-readable summary, surfaced in logs.
	description string
	// settings maps a fully-qualified config key (e.g.
	// "logs_config.pipelines") to the value the profile forces.
	settings map[string]interface{}
}

// logsPerformanceProfiles is the catalog of available profiles, keyed by
// profile name and then by version.
//
// IMPORTANT: published (name, version) pairs are IMMUTABLE. Never change the
// settings of an already-released version. Improved tuning must ship as a NEW
// version number so that upgrading the agent can never silently change a
// host's effective configuration. A bare `logs_config.profile: <name>` (no
// version) always resolves to version 1, so version 1 must always exist for
// every published profile.
//
// The values below are v1 baselines intended to be calibrated with
// benchmarking data; once a version is released its values must not change.
var logsPerformanceProfiles = map[string]map[int]logsPerformanceProfile{
	// high-throughput maximizes sustained log volume: more pipelines, higher
	// send concurrency, and larger in-memory buffers.
	"high-throughput": {
		1: {
			description: "Maximize sustained log throughput (more pipelines, higher send concurrency, larger buffers).",
			settings: map[string]interface{}{
				"logs_config.pipelines":                 8,
				"logs_config.batch_max_concurrent_send": 10,
				"logs_config.message_channel_size":      200,
				"logs_config.payload_channel_size":      20,
				"logs_config.use_compression":           true,
				"logs_config.compression_kind":          "zstd",
			},
		},
	},
	// low-latency minimizes delivery delay: small batch wait, smaller batches,
	// higher send concurrency.
	"low-latency": {
		1: {
			description: "Minimize log delivery latency (short batch wait, smaller batches, higher send concurrency).",
			settings: map[string]interface{}{
				"logs_config.batch_wait":                1.0,
				"logs_config.batch_max_size":            500,
				"logs_config.batch_max_concurrent_send": 10,
			},
		},
	},
	// low-resource minimizes CPU and memory footprint: fewer pipelines, lower
	// concurrency, smaller buffers.
	"low-resource": {
		1: {
			description: "Minimize CPU and memory footprint (fewer pipelines, lower concurrency, smaller buffers).",
			settings: map[string]interface{}{
				"logs_config.pipelines":                 2,
				"logs_config.batch_max_concurrent_send": 1,
				"logs_config.message_channel_size":      50,
				"logs_config.payload_channel_size":      5,
			},
		},
	},
}

// LogsPerformanceProfileSetting is a single config key/value that a profile
// applies. Used to describe the active profile (e.g. in the agent status).
type LogsPerformanceProfileSetting struct {
	Key   string
	Value interface{}
}

// isLogsPerformanceProfileOff reports whether the given profile name disables
// profiles (empty or one of the explicit "off" aliases).
func isLogsPerformanceProfileOff(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "off", "none", "default":
		return true
	}
	return false
}

// resolveLogsPerformanceProfileVersion maps an unset (0) version to 1 for
// upgrade-safety; otherwise returns the version unchanged.
func resolveLogsPerformanceProfileVersion(version int) int {
	if version == 0 {
		return 1
	}
	return version
}

// lookupLogsPerformanceProfile resolves the profile selected by config to its
// definition, name, and version. ok is false when no valid profile is active.
func lookupLogsPerformanceProfile(config pkgconfigmodel.Reader) (profile logsPerformanceProfile, name string, version int, ok bool) {
	name = config.GetString("logs_config.profile")
	if isLogsPerformanceProfileOff(name) {
		return logsPerformanceProfile{}, name, 0, false
	}
	versions, found := logsPerformanceProfiles[name]
	if !found {
		return logsPerformanceProfile{}, name, 0, false
	}
	version = resolveLogsPerformanceProfileVersion(config.GetInt("logs_config.profile_version"))
	profile, found = versions[version]
	if !found {
		return logsPerformanceProfile{}, name, version, false
	}
	return profile, name, version, true
}

// ResolvedLogsPerformanceProfile reports the logs performance profile in effect
// for the given config: its name, resolved version, and the settings it applies
// (sorted by key). ok is false when no valid profile is active. This is a
// read-only view intended for diagnostics such as the agent status output.
func ResolvedLogsPerformanceProfile(config pkgconfigmodel.Reader) (name string, version int, settings []LogsPerformanceProfileSetting, ok bool) {
	profile, name, version, ok := lookupLogsPerformanceProfile(config)
	if !ok {
		return "", 0, nil, false
	}
	for _, key := range sortedSettingKeys(profile.settings) {
		settings = append(settings, LogsPerformanceProfileSetting{Key: key, Value: profile.settings[key]})
	}
	return name, version, settings, true
}

// LogsPerformanceProfileExists reports whether a profile with the given name is
// defined in the catalog (in any version). Useful for validating a recommended
// profile name without a config object.
func LogsPerformanceProfileExists(name string) bool {
	_, ok := logsPerformanceProfiles[name]
	return ok
}

// applyLogsPerformanceProfile expands the logs_config.profile selection into
// the underlying logs_config.* settings. It is registered as a config override
// func, so it runs after datadog.yaml and environment variables have been
// merged but before the logs agent reads any of these values.
//
// Profile values are written at SourceConfigPostInit, which takes precedence
// over file and environment sources ("the profile wins") but still yields to
// higher-priority sources such as remote-config and live CLI overrides.
func applyLogsPerformanceProfile(config pkgconfigmodel.Config) {
	name := config.GetString("logs_config.profile")
	// Empty (or the explicit "off"/"default"/"none" aliases) means profiles
	// are disabled and the agent keeps its normal default settings.
	if isLogsPerformanceProfileOff(name) {
		return
	}

	versions, ok := logsPerformanceProfiles[name]
	if !ok {
		log.Errorf("Unknown logs_config.profile %q; ignoring and using default logs settings. Available profiles: %s",
			name, strings.Join(sortedProfileNames(), ", "))
		return
	}

	version := resolveLogsPerformanceProfileVersion(config.GetInt("logs_config.profile_version"))

	profile, ok := versions[version]
	if !ok {
		log.Errorf("Unknown version %d for logs_config.profile %q; ignoring and using default logs settings. Available versions: %s",
			version, name, sortedVersions(versions))
		return
	}

	var overridden []string
	for _, key := range sortedSettingKeys(profile.settings) {
		value := profile.settings[key]
		// "Profile wins + warn": if the user explicitly configured this key,
		// the profile still overrides it, but we surface that so the behavior
		// is observable.
		if config.IsConfigured(key) {
			overridden = append(overridden, fmt.Sprintf("%s (was %v, now %v)", key, config.Get(key), value))
		}
		config.Set(key, value, pkgconfigmodel.SourceConfigPostInit)
	}

	log.Infof("Applied logs performance profile %q (version %d): %s", name, version, profile.description)
	if len(overridden) > 0 {
		log.Warnf("Logs performance profile %q (version %d) overrode explicitly-configured settings: %s",
			name, version, strings.Join(overridden, ", "))
	}
}

func sortedProfileNames() []string {
	names := make([]string, 0, len(logsPerformanceProfiles))
	for name := range logsPerformanceProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedVersions(versions map[int]logsPerformanceProfile) string {
	nums := make([]int, 0, len(versions))
	for version := range versions {
		nums = append(nums, version)
	}
	sort.Ints(nums)
	parts := make([]string, 0, len(nums))
	for _, n := range nums {
		parts = append(parts, fmt.Sprintf("%d", n))
	}
	return strings.Join(parts, ", ")
}

func sortedSettingKeys(settings map[string]interface{}) []string {
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
