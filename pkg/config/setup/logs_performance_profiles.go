// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// logsPerformanceProfile is a pre-tuned bundle of logs_config.* settings.
type logsPerformanceProfile struct {
	description string                 // human-readable summary, surfaced in logs
	settings    map[string]interface{} // config key -> forced value
}

// pipelineCountSentinel is a logs_config.pipelines value resolved against the
// host CPU count at apply time: max==0 means one per core (uncapped), max>0
// means min(max, cores) so a profile never raises the count on a small host.
type pipelineCountSentinel struct{ max int }

// pipelinesPerCore resolves to one pipeline per logical CPU, uncapped.
var pipelinesPerCore = pipelineCountSentinel{}

// pipelinesAtMost resolves to min(n, GOMAXPROCS).
func pipelinesAtMost(n int) pipelineCountSentinel { return pipelineCountSentinel{max: n} }

// resolveProfileSettingValue resolves a sentinel setting value; plain values pass through.
func resolveProfileSettingValue(v interface{}) interface{} {
	if s, ok := v.(pipelineCountSentinel); ok {
		cores := runtime.GOMAXPROCS(0)
		if s.max > 0 && s.max < cores {
			return s.max
		}
		return cores
	}
	return v
}

// mergeProfileSettings returns the union of the given maps; later maps win.
func mergeProfileSettings(maps ...map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// Throughput profiles form a monotonic ladder, each a superset of the previous,
// so escalating never lowers a setting a lower tier raised:
// high-concurrency ⊂ high-throughput ⊂ max-throughput.
var (
	highConcurrencySettings = map[string]interface{}{
		"logs_config.batch_max_concurrent_send": 20,
		"logs_config.payload_channel_size":      40,
	}
	highThroughputSettings = mergeProfileSettings(highConcurrencySettings, map[string]interface{}{
		"logs_config.pipelines":            pipelinesPerCore,
		"logs_config.message_channel_size": 200,
	})
	maxThroughputSettings = mergeProfileSettings(highThroughputSettings, map[string]interface{}{
		"logs_config.use_compression": false,
	})
)

// logsPerformanceProfiles is the catalog, keyed by name then version.
//
// Published (name, version) pairs are IMMUTABLE: improved tuning must ship as a
// new version so upgrades never silently change a host's config. A bare profile
// name resolves to version 1, so version 1 must always exist.
var logsPerformanceProfiles = map[string]map[int]logsPerformanceProfile{
	"high-concurrency": {
		1: {
			description: "Maximize concurrent in-flight sends to absorb intake/network latency (send/transport bottleneck).",
			settings:    highConcurrencySettings,
		},
	},
	"high-throughput": {
		1: {
			description: "Maximize sustained log throughput (one pipeline per core, high send concurrency, larger buffers).",
			settings:    highThroughputSettings,
		},
	},
	"max-throughput": {
		1: {
			description: "Remove the compression CPU bottleneck (high-throughput plus compression disabled), at the expense of bandwidth and memory.",
			settings:    maxThroughputSettings,
		},
	},
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
	"low-resource": {
		1: {
			description: "Minimize CPU and memory footprint (fewer pipelines, lower concurrency, smaller buffers).",
			settings: map[string]interface{}{
				"logs_config.pipelines":                 pipelinesAtMost(2),
				"logs_config.batch_max_concurrent_send": 1,
				"logs_config.message_channel_size":      50,
				"logs_config.payload_channel_size":      5,
			},
		},
	},
	"high-compression": {
		1: {
			description: "Reduce network bytes at the cost of CPU (higher compression level, larger payloads).",
			settings: map[string]interface{}{
				"logs_config.use_compression":        true,
				"logs_config.compression_kind":       "zstd",
				"logs_config.zstd_compression_level": 6,
				"logs_config.batch_max_content_size": 5000000,
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

// ResolvedLogsPerformanceProfile reports the active profile's name, version, and
// settings (sorted by key). ok is false when no valid profile is active.
func ResolvedLogsPerformanceProfile(config pkgconfigmodel.Reader) (name string, version int, settings []LogsPerformanceProfileSetting, ok bool) {
	profile, name, version, ok := lookupLogsPerformanceProfile(config)
	if !ok {
		return "", 0, nil, false
	}
	for _, key := range sortedSettingKeys(profile.settings) {
		settings = append(settings, LogsPerformanceProfileSetting{Key: key, Value: resolveProfileSettingValue(profile.settings[key])})
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

// LogsPerformanceProfileCovers reports whether active is a superset of candidate
// (applies every candidate setting at the same value), so callers can avoid
// recommending a switch that would only lower settings. Compares version 1.
func LogsPerformanceProfileCovers(active, candidate string) bool {
	if isLogsPerformanceProfileOff(active) || isLogsPerformanceProfileOff(candidate) {
		return false
	}
	a, ok := logsPerformanceProfiles[active][1]
	if !ok {
		return false
	}
	c, ok := logsPerformanceProfiles[candidate][1]
	if !ok {
		return false
	}
	for key, candidateVal := range c.settings {
		activeVal, ok := a.settings[key]
		if !ok || resolveProfileSettingValue(activeVal) != resolveProfileSettingValue(candidateVal) {
			return false
		}
	}
	return true
}

// profileYieldSources are the sources a profile must not override: anything the
// user, a policy, or runtime set, as opposed to defaults or the profile's own
// SourceConfigPostInit writes. This lets a customer pick a profile and still
// tweak individual logs_config.* knobs on top of it.
var profileYieldSources = map[pkgconfigmodel.Source]struct{}{
	pkgconfigmodel.SourceFile:               {},
	pkgconfigmodel.SourceEnvVar:             {},
	pkgconfigmodel.SourceFleetPolicies:      {},
	pkgconfigmodel.SourceSecret:             {},
	pkgconfigmodel.SourceLocalConfigProcess: {},
	pkgconfigmodel.SourceAgentRuntime:       {},
	pkgconfigmodel.SourceRC:                 {},
	pkgconfigmodel.SourceCLI:                {},
}

// keyExplicitlySet reports whether key has a value from a source the profile
// should yield to, read per-source so the profile's own write does not mask it.
func keyExplicitlySet(config pkgconfigmodel.Reader, key string) bool {
	for _, vs := range config.GetAllSources(key) {
		if vs.Value == nil {
			continue
		}
		if _, ok := profileYieldSources[vs.Source]; ok {
			return true
		}
	}
	return false
}

// allProfileSettingKeys returns the union of every config key any catalog profile
// can write, sorted. Used to clear a previous apply's writes before re-evaluating.
func allProfileSettingKeys() []string {
	seen := map[string]struct{}{}
	for _, versions := range logsPerformanceProfiles {
		for _, profile := range versions {
			for key := range profile.settings {
				seen[key] = struct{}{}
			}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ApplyLogsPerformanceProfile expands logs_config.profile into the underlying
// logs_config.* settings, writing at SourceConfigPostInit. It yields to any key
// the user, a policy, or runtime set, so explicit settings win over the profile.
//
// It is idempotent: each call first clears its own prior SourceConfigPostInit
// writes, so it can be safely re-run after additional config sources merge. The
// full agent merges fleet policies only after the initial override pass, so this
// must run again afterward (see comp/core/config) — otherwise a profile selected
// by fleet policy is never expanded, and the first pass's post-init writes (which
// outrank SourceFleetPolicies) would shadow a fleet-policy knob override that is
// meant to win over the profile.
func ApplyLogsPerformanceProfile(config pkgconfigmodel.Config) {
	// Drop any values a previous apply wrote so this pass reflects the current
	// config state instead of stacking on top of, or being shadowed by, stale
	// post-init writes.
	for _, key := range allProfileSettingKeys() {
		config.UnsetForSource(key, pkgconfigmodel.SourceConfigPostInit)
	}

	name := config.GetString("logs_config.profile")
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

	var kept []string
	for _, key := range sortedSettingKeys(profile.settings) {
		// An explicitly-set key wins over the profile.
		if keyExplicitlySet(config, key) {
			kept = append(kept, fmt.Sprintf("%s (%v)", key, config.Get(key)))
			continue
		}
		config.Set(key, resolveProfileSettingValue(profile.settings[key]), pkgconfigmodel.SourceConfigPostInit)
	}

	log.Infof("Applied logs performance profile %q (version %d): %s", name, version, profile.description)
	if len(kept) > 0 {
		log.Infof("Logs performance profile %q (version %d) kept explicitly-configured settings: %s",
			name, version, strings.Join(kept, ", "))
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
		parts = append(parts, strconv.Itoa(n))
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
