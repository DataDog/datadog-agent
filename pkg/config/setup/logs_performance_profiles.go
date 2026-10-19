// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// logsPerformanceProfile is a pre-tuned bundle of logs_config.* settings.
type logsPerformanceProfile struct {
	description string                 // human-readable summary, surfaced in logs
	settings    map[string]interface{} // config key -> forced value
}

// pipelineCountSentinel resolves to min(max, cores) at apply time.
type pipelineCountSentinel struct{ max int }

// pipelinesAtMost resolves to min(n, GOMAXPROCS).
func pipelinesAtMost(n int) pipelineCountSentinel { return pipelineCountSentinel{max: n} }

// resolveProfileSettingValue resolves a sentinel setting value; plain values pass through.
func resolveProfileSettingValue(v interface{}) interface{} {
	if s, ok := v.(pipelineCountSentinel); ok {
		return min(s.max, runtime.GOMAXPROCS(0))
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

// Throughput profiles are supersets: high-concurrency ⊂ high-throughput ⊂ max-throughput.
var (
	highConcurrencySettings = map[string]interface{}{
		"logs_config.batch_max_concurrent_send": 20,
		"logs_config.payload_channel_size":      40,
	}
	highThroughputSettings = mergeProfileSettings(highConcurrencySettings, map[string]interface{}{
		"logs_config.pipelines":            pipelinesAtMost(16),
		"logs_config.message_channel_size": 200,
	})
	maxThroughputSettings = mergeProfileSettings(highThroughputSettings, map[string]interface{}{
		"logs_config.use_compression": false,
	})
)

// logsPerformanceProfiles is the catalog, keyed by name.
var logsPerformanceProfiles = map[string]logsPerformanceProfile{
	"high-concurrency": {
		description: "Maximize concurrent in-flight sends to absorb intake/network latency (send/transport bottleneck).",
		settings:    highConcurrencySettings,
	},
	"high-throughput": {
		description: "Maximize sustained log throughput (one pipeline per core up to 16, high send concurrency, larger buffers).",
		settings:    highThroughputSettings,
	},
	"max-throughput": {
		description: "Remove the compression CPU bottleneck (high-throughput plus compression disabled, including for HTTP additional_endpoints), at the expense of bandwidth and memory.",
		settings:    maxThroughputSettings,
	},
	"low-latency": {
		description: "Minimize log delivery latency (short batch wait).",
		settings: map[string]interface{}{
			"logs_config.batch_wait": 1.0,
		},
	},
	"low-resource": {
		description: "Minimize CPU and memory footprint (fewer pipelines, lower send concurrency, fewer in-flight payloads).",
		settings: map[string]interface{}{
			"logs_config.pipelines":                 pipelinesAtMost(2),
			"logs_config.batch_max_concurrent_send": 4,
			"logs_config.payload_channel_size":      5,
		},
	},
	"high-compression": {
		description: "Reduce network bytes at the cost of CPU (higher zstd compression level; no effect when logs are sent with gzip, the default with additional_endpoints).",
		settings: map[string]interface{}{
			"logs_config.zstd_compression_level": 6,
		},
	},
}

// LogsPerformanceProfileSetting is one config key/value a profile applies.
type LogsPerformanceProfileSetting struct {
	Key   string
	Value interface{}
}

// isLogsPerformanceProfileOff reports whether name is empty or an "off" alias.
func isLogsPerformanceProfileOff(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "off", "none", "default":
		return true
	}
	return false
}

// configuredLogsPerformanceProfile returns logs_config.profile trimmed and lowercased.
func configuredLogsPerformanceProfile(config pkgconfigmodel.Reader) string {
	return strings.ToLower(strings.TrimSpace(config.GetString("logs_config.profile")))
}

// lookupLogsPerformanceProfile resolves the configured profile; ok is false when none is active.
func lookupLogsPerformanceProfile(config pkgconfigmodel.Reader) (profile logsPerformanceProfile, name string, ok bool) {
	name = configuredLogsPerformanceProfile(config)
	if isLogsPerformanceProfileOff(name) {
		return logsPerformanceProfile{}, name, false
	}
	profile, ok = logsPerformanceProfiles[name]
	return profile, name, ok
}

// ResolvedLogsPerformanceProfile reports the active profile and its settings sorted by key.
func ResolvedLogsPerformanceProfile(config pkgconfigmodel.Reader) (name string, settings []LogsPerformanceProfileSetting, ok bool) {
	profile, name, ok := lookupLogsPerformanceProfile(config)
	if !ok {
		return "", nil, false
	}
	for _, key := range sortedSettingKeys(profile.settings) {
		settings = append(settings, LogsPerformanceProfileSetting{Key: key, Value: resolveProfileSettingValue(profile.settings[key])})
	}
	return name, settings, true
}

// LogsPerformanceProfileExists reports whether name is in the catalog.
func LogsPerformanceProfileExists(name string) bool {
	_, ok := logsPerformanceProfiles[name]
	return ok
}

// LogsPerformanceProfileCovers reports whether active applies every candidate setting at the same value.
func LogsPerformanceProfileCovers(active, candidate string) bool {
	if isLogsPerformanceProfileOff(active) || isLogsPerformanceProfileOff(candidate) {
		return false
	}
	a, ok := logsPerformanceProfiles[active]
	if !ok {
		return false
	}
	c, ok := logsPerformanceProfiles[candidate]
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

// profileYieldSources are the sources a profile never overrides.
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

// keyExplicitlySet reports whether key is set by a source in profileYieldSources.
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

// LogsPerformanceProfileChange is a key whose effective value would change.
type LogsPerformanceProfileChange struct {
	Key  string
	From interface{}
	To   interface{}
}

// LogsPerformanceProfileBlockedKey is a profile key that an explicit setting overrides.
type LogsPerformanceProfileBlockedKey struct {
	Key    string
	Source string
}

// LogsPerformanceProfileCurrentSetting is a profile key's effective value and source today.
type LogsPerformanceProfileCurrentSetting struct {
	Key    string
	Value  interface{}
	Source string
}

// LogsPerformanceProfilePlan describes what switching to a profile would change.
type LogsPerformanceProfilePlan struct {
	Name             string
	Version          int
	Description      string
	Current          []LogsPerformanceProfileCurrentSetting
	Changes          []LogsPerformanceProfileChange
	Blocked          []LogsPerformanceProfileBlockedKey
	ProfileKeySource string
}

// LatestLogsPerformanceProfileVersion returns the version new deployments should pin for name.
func LatestLogsPerformanceProfileVersion(name string) (int, bool) {
	versions, ok := logsPerformanceProfiles[name]
	if !ok || len(versions) == 0 {
		return 0, false
	}
	latest := 0
	for v := range versions {
		latest = max(latest, v)
	}
	return latest, true
}

// PlanLogsPerformanceProfile describes what switching to candidate (latest version) would change on this config. ok is false if candidate is not in the catalog.
func PlanLogsPerformanceProfile(config pkgconfigmodel.Reader, candidate string) (LogsPerformanceProfilePlan, bool) {
	version, ok := LatestLogsPerformanceProfileVersion(candidate)
	if !ok {
		return LogsPerformanceProfilePlan{}, false
	}
	profile := logsPerformanceProfiles[candidate][version]
	plan := LogsPerformanceProfilePlan{
		Name:             candidate,
		Version:          version,
		Description:      profile.description,
		ProfileKeySource: string(config.GetSource("logs_config.profile")),
	}
	for _, key := range sortedSettingKeys(profile.settings) {
		current := config.Get(key)
		plan.Current = append(plan.Current, LogsPerformanceProfileCurrentSetting{
			Key:    key,
			Value:  current,
			Source: string(config.GetSource(key)),
		})
		if keyExplicitlySet(config, key) {
			plan.Blocked = append(plan.Blocked, LogsPerformanceProfileBlockedKey{Key: key, Source: string(explicitSource(config, key))})
			continue
		}
		target := resolveProfileSettingValue(profile.settings[key])
		if !profileValuesEqual(current, target) {
			plan.Changes = append(plan.Changes, LogsPerformanceProfileChange{Key: key, From: current, To: target})
		}
	}
	return plan, true
}

// explicitSource returns the source that makes key explicitly set.
func explicitSource(config pkgconfigmodel.Reader, key string) pkgconfigmodel.Source {
	if src := config.GetSource(key); isYieldSource(src) {
		return src
	}
	for _, vs := range config.GetAllSources(key) {
		if vs.Value != nil && isYieldSource(vs.Source) {
			return vs.Source
		}
	}
	return config.GetSource(key)
}

func isYieldSource(src pkgconfigmodel.Source) bool {
	_, ok := profileYieldSources[src]
	return ok
}

// profileValuesEqual compares config values, treating numeric types as equal when numerically equal.
func profileValuesEqual(a, b interface{}) bool {
	if fa, ok := toFloat64(a); ok {
		fb, ok := toFloat64(b)
		return ok && fa == fb
	}
	return reflect.DeepEqual(a, b)
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// allProfileSettingKeys returns every key any profile can write, sorted.
func allProfileSettingKeys() []string {
	seen := map[string]struct{}{}
	for _, profile := range logsPerformanceProfiles {
		for key := range profile.settings {
			seen[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ApplyLogsPerformanceProfile expands logs_config.profile at SourceConfigPostInit; it is idempotent.
func ApplyLogsPerformanceProfile(config pkgconfigmodel.Config) {
	for _, key := range allProfileSettingKeys() {
		config.UnsetForSource(key, pkgconfigmodel.SourceConfigPostInit)
	}

	name := configuredLogsPerformanceProfile(config)
	if isLogsPerformanceProfileOff(name) {
		return
	}

	profile, ok := logsPerformanceProfiles[name]
	if !ok {
		log.Errorf("Unknown logs_config.profile %q; ignoring and using default logs settings. Available profiles: %s",
			name, strings.Join(sortedProfileNames(), ", "))
		return
	}

	var kept []string
	for _, key := range sortedSettingKeys(profile.settings) {
		if keyExplicitlySet(config, key) {
			kept = append(kept, fmt.Sprintf("%s (%v)", key, config.Get(key)))
			continue
		}
		config.Set(key, resolveProfileSettingValue(profile.settings[key]), pkgconfigmodel.SourceConfigPostInit)
	}

	log.Infof("Applied logs performance profile %q: %s", name, profile.description)
	if _, ok := profile.settings[zstdLevelKey]; ok && !keyExplicitlySet(config, zstdLevelKey) {
		if reason := zstdLevelIgnoredReason(config); reason != "" {
			log.Warnf("Logs performance profile %q sets %s, which has no effect: %s", name, zstdLevelKey, reason)
		}
	}
	if len(kept) > 0 {
		log.Infof("Logs performance profile %q kept explicitly-configured settings: %s",
			name, strings.Join(kept, ", "))
	}
}

const zstdLevelKey = "logs_config.zstd_compression_level"

// zstdLevelIgnoredReason explains why zstd_compression_level has no effect, or returns "" when it applies.
func zstdLevelIgnoredReason(config pkgconfigmodel.Reader) string {
	switch {
	case !config.GetBool("logs_config.use_compression"):
		return "logs_config.use_compression is false"
	case config.IsConfigured("logs_config.compression_kind"):
		if kind := config.GetString("logs_config.compression_kind"); kind != "zstd" {
			return fmt.Sprintf("logs_config.compression_kind is %q", kind)
		}
		return ""
	case hasAdditionalLogsEndpoints(config):
		return "additional_endpoints fall back to gzip; set logs_config.compression_kind to zstd to use it"
	}
	return ""
}

// hasAdditionalLogsEndpoints reports whether logs_config.additional_endpoints is non-empty, as a list or a JSON string.
func hasAdditionalLogsEndpoints(config pkgconfigmodel.Reader) bool {
	switch v := config.Get("logs_config.additional_endpoints").(type) {
	case []interface{}:
		return len(v) > 0
	case string:
		var endpoints []interface{}
		return json.Unmarshal([]byte(v), &endpoints) == nil && len(endpoints) > 0
	}
	return false
}

func sortedProfileNames() []string {
	names := make([]string, 0, len(logsPerformanceProfiles))
	for name := range logsPerformanceProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedSettingKeys(settings map[string]interface{}) []string {
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
