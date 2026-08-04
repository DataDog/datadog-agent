// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package preflightmodeimpl

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// get looks up a dotted key in a nested map, reporting whether it was present.
func get(t *testing.T, m map[string]any, key string) (any, bool) {
	t.Helper()
	cur := any(m)
	for _, part := range splitDots(key) {
		asMap, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = asMap[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func splitDots(key string) []string {
	var out []string
	start := 0
	for i := 0; i < len(key); i++ {
		if key[i] == '.' {
			out = append(out, key[start:i])
			start = i + 1
		}
	}
	return append(out, key[start:])
}

func requireEq(t *testing.T, m map[string]any, key string, want any) {
	t.Helper()
	got, ok := get(t, m, key)
	require.Truef(t, ok, "%s is missing from the generated config", key)
	assert.Equalf(t, want, got, "%s", key)
}

func TestBuildPreflightConfigCarriesOperatorSettings(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("api_key", "0123456789abcdef0123456789abcdef", pkgconfigmodel.SourceFile)
	cfg.Set("site", "datadoghq.eu", pkgconfigmodel.SourceFile)
	cfg.Set("proxy.https", "http://proxy.internal:3128", pkgconfigmodel.SourceFile)

	got := buildPreflightConfig(cfg, newListener(t.TempDir()))

	// The whole premise of the pre-flight is that ADP runs against the operator's real
	// configuration, api_key and proxy included.
	requireEq(t, got, "api_key", "0123456789abcdef0123456789abcdef")
	requireEq(t, got, "site", "datadoghq.eu")
	requireEq(t, got, "proxy.https", "http://proxy.internal:3128")
}

// TestBuildPreflightConfigCarriesTheWholeAgentConfig documents the deliberate choice to base
// the preflight config on AllSettings, defaults included, rather than only the settings the
// operator touched.
//
// The point of the pre-flight is for ADP to see the configuration it would really run with,
// and passing everything through is the closest approximation. It is safe because ADP
// ignores keys it does not recognise: verified against agent-data-plane 1.4.0 by running it
// with the full Core Agent config, including Core-Agent-only sections, with no resulting
// warning or error (see TestBuildPreflightConfigPassesThroughCoreAgentOnlySettings).
func TestBuildPreflightConfigCarriesTheWholeAgentConfig(t *testing.T) {
	cfg := configmock.New(t)

	got := buildPreflightConfig(cfg, newListener(t.TempDir()))

	// Defaults ADP shares with the Agent come through, so it forwards the way the Agent
	// would.
	_, present := get(t, got, "forwarder_timeout")
	assert.True(t, present, "the Agent's forwarder settings should reach ADP")
	_, present = get(t, got, "site")
	assert.True(t, present, "the Agent's site should reach ADP")
}

func TestBuildPreflightConfigOverrides(t *testing.T) {
	cfg := configmock.New(t)
	workDir := t.TempDir()

	got := buildPreflightConfig(cfg, newListener(workDir))

	requireEq(t, got, DataPlaneEnabled, true)
	requireEq(t, got, "data_plane.dogstatsd.enabled", true)
	requireEq(t, got, "data_plane.otlp.enabled", false)

	// Standalone mode is what keeps the preflight process from talking to the Core Agent at
	// all, for either registration or configuration.
	requireEq(t, got, "data_plane.standalone_mode", true)
	requireEq(t, got, "data_plane.remote_agent_enabled", false)
	requireEq(t, got, "data_plane.use_new_config_stream_endpoint", false)

	requireEq(t, got, "data_plane.api_listen_address", "tcp://127.0.0.1:0")
	requireEq(t, got, "data_plane.secure_api_listen_address", "tcp://127.0.0.1:0")
	requireEq(t, got, "data_plane.telemetry_enabled", false)

	// Console-only JSON logging is what makes the output scannable without ADP writing
	// anywhere near the real agent-data-plane.log.
	requireEq(t, got, "disable_file_logging", true)
	requireEq(t, got, "log_to_console", true)
	requireEq(t, got, "log_format_json", true)
	requireEq(t, got, "log_level", "info")

	// No UDP listener, and no second unix/pipe listener.
	requireEq(t, got, "dogstatsd_port", 0)
	requireEq(t, got, "dogstatsd_stream_socket", "")
	requireEq(t, got, "dogstatsd_non_local_traffic", false)
}

// TestBuildPreflightConfigOverridesOperatorLogging matters because the operator's own logging
// settings would otherwise defeat the scan: a file-logging ADP would write next to the real
// agent-data-plane.log, a non-JSON ADP would not be parseable, and a debug-level ADP would
// swamp the capture buffer.
func TestBuildPreflightConfigOverridesOperatorLogging(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("disable_file_logging", false, pkgconfigmodel.SourceFile)
	cfg.Set("log_format_json", false, pkgconfigmodel.SourceFile)
	cfg.Set("log_to_console", false, pkgconfigmodel.SourceFile)
	cfg.Set("log_level", "debug", pkgconfigmodel.SourceFile)

	got := buildPreflightConfig(cfg, newListener(t.TempDir()))

	requireEq(t, got, "disable_file_logging", true)
	requireEq(t, got, "log_format_json", true)
	requireEq(t, got, "log_to_console", true)
	requireEq(t, got, "log_level", "info")
}

// TestOverrideKeysAreKnown is the guard that makes a whole class of bug impossible.
//
// Every override is applied with setNested straight into the emitted map, so a misspelled
// key is completely silent: it writes something ADP ignores, while the setting it was meant
// to override passes through from AllSettings untouched. That is not hypothetical — this
// change originally overrode "dogstatsd_metric_namespace", which is not an Agent key (the
// real one is statsd_metric_namespace), so the operator's namespace reached ADP and
// prefixed the probe metric, defeating its n_o_i_n_d_e_x. prefix and making it an indexed,
// billed custom metric.
//
// A test asserting the override landed cannot catch this, because the override always
// lands. Only checking the key against the Agent's schema can.
//
// preflightModeDataPlaneOnlyOverrides is deliberately excluded: those keys exist in ADP but not
// in the Agent, so there is nothing to check them against. That is exactly why that map is
// kept as short as possible.
func TestOverrideKeysAreKnown(t *testing.T) {
	cfg := configmock.New(t)

	keys := make([]string, 0, len(preflightModeGlobalOverrides))
	for k := range preflightModeGlobalOverrides {
		keys = append(keys, k)
	}
	for k := range newListener(t.TempDir()).configOverrides() {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		assert.Truef(t, cfg.IsKnown(k),
			"%q is not a known Agent config setting, so overriding it is a silent no-op", k)
	}
}

// TestDataPlaneOnlyOverridesStaySmall keeps the unvalidatable set from quietly growing. Each
// entry is a key nothing in this repo can check, so adding one is a deliberate decision that
// should show up in review.
func TestDataPlaneOnlyOverridesStaySmall(t *testing.T) {
	cfg := configmock.New(t)

	assert.Len(t, preflightModeDataPlaneOnlyOverrides, 1,
		"adding an unvalidatable override needs a note on why it cannot live in preflightModeGlobalOverrides")
	for k := range preflightModeDataPlaneOnlyOverrides {
		assert.Falsef(t, cfg.IsKnown(k),
			"%q IS a known Agent setting, so it belongs in preflightModeGlobalOverrides where it gets validated", k)
	}
}

// TestBuildPreflightConfigClearsMetricNamespace guards the n_o_i_n_d_e_x. prefix: DogStatsD
// prepends statsd_metric_namespace to every metric name (comp/dogstatsd/server/impl/enrich.go),
// which would turn the probe into an indexed metric in the customer's account.
func TestBuildPreflightConfigClearsMetricNamespace(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("statsd_metric_namespace", "acme.", pkgconfigmodel.SourceFile)
	cfg.Set("statsd_metric_namespace_blacklist", []string{"datadog.agent"}, pkgconfigmodel.SourceFile)

	// Assert the Given clause actually took. Set on an unknown key is a silent no-op, which
	// is how the original version of this test passed while guarding nothing.
	require.Equal(t, "acme.", cfg.GetString("statsd_metric_namespace"))

	got := buildPreflightConfig(cfg, newListener(t.TempDir()))

	requireEq(t, got, "statsd_metric_namespace", "")
	requireEq(t, got, "statsd_metric_namespace_blacklist", []string{})
}

// TestBuildPreflightConfigDisablesDiskRetryQueue guards the isolation of ADP's on-disk footprint.
//
// With the retry queue enabled, ADP creates directories under forwarder_storage_path and
// persists transactions there — the Core Agent's own retry tree, outside the working directory
// the run secures and deletes. Zeroing the size is what stops ADP taking that branch at all;
// see the override's comment for why clearing the path instead does not work.
func TestBuildPreflightConfigDisablesDiskRetryQueue(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("forwarder_storage_max_size_in_bytes", 500_000_000, pkgconfigmodel.SourceFile)

	// Set on an unknown key is a silent no-op, so prove the Given clause took.
	require.Equal(t, int64(500_000_000), cfg.GetInt64("forwarder_storage_max_size_in_bytes"))

	got := buildPreflightConfig(cfg, newListener(t.TempDir()))

	requireEq(t, got, "forwarder_storage_max_size_in_bytes", 0)
}

// TestBuildPreflightConfigOverridesOperatorListeners is the safety property that matters most:
// whatever the operator configured, the preflight process must never end up on the real
// DogStatsD endpoints.
func TestBuildPreflightConfigOverridesOperatorListeners(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("dogstatsd_port", 8125, pkgconfigmodel.SourceFile)
	cfg.Set("dogstatsd_socket", "/var/run/datadog/dsd.socket", pkgconfigmodel.SourceFile)
	cfg.Set("dogstatsd_stream_socket", "/var/run/datadog/dsd-stream.socket", pkgconfigmodel.SourceFile)
	cfg.Set("dogstatsd_pipe_name", "dogstatsd-real", pkgconfigmodel.SourceFile)
	cfg.Set("dogstatsd_non_local_traffic", true, pkgconfigmodel.SourceFile)

	workDir := t.TempDir()
	l := newListener(workDir)
	got := buildPreflightConfig(cfg, l)

	requireEq(t, got, "dogstatsd_port", 0)
	requireEq(t, got, "dogstatsd_stream_socket", "")
	requireEq(t, got, "dogstatsd_non_local_traffic", false)

	if runtime.GOOS == "windows" {
		pipe, _ := get(t, got, "dogstatsd_pipe_name")
		assert.NotEqual(t, "dogstatsd-real", pipe)
		requireEq(t, got, "dogstatsd_socket", "")
	} else {
		requireEq(t, got, "dogstatsd_socket", filepath.Join(workDir, "dsd.socket"))
		requireEq(t, got, "dogstatsd_pipe_name", "")
	}
}

// TestBuildPreflightConfigPassesThroughCoreAgentOnlySettings pins the behaviour that Core
// Agent-only settings are handed to ADP untouched rather than stripped.
//
// This is safe, and was checked against the real binary rather than assumed:
// agent-data-plane 1.4.0 given a config containing data_plane.preflight_mode*, a full
// otlp_config section, apm_config, process_config and logs_enabled started cleanly, logged
// no warning about any of them, and bound neither 4317 nor 4318 — ADP drives its OTLP
// surfaces from data_plane.otlp.*, not from the Core Agent's otlp_config.
//
// If a future ADP starts rejecting unknown keys, or starts honouring otlp_config, this test
// is the place to reintroduce stripping.
func TestBuildPreflightConfigPassesThroughCoreAgentOnlySettings(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set(DataPlanePreflightMode, true, pkgconfigmodel.SourceFile)
	cfg.Set("otlp_config.receiver.protocols.grpc.endpoint", "0.0.0.0:4317", pkgconfigmodel.SourceFile)

	got := buildPreflightConfig(cfg, newListener(t.TempDir()))

	_, present := get(t, got, DataPlanePreflightMode)
	assert.True(t, present, "ADP ignores keys it does not recognise, so stripping is unnecessary")
	_, present = get(t, got, "otlp_config")
	assert.True(t, present, "ADP does not act on the Core Agent's otlp_config section")
}

func TestWritePreflightConfig(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("api_key", "0123456789abcdef0123456789abcdef", pkgconfigmodel.SourceFile)

	workDir := t.TempDir()
	path, err := writePreflightConfig(cfg, newListener(workDir), workDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(workDir, preflightConfigFileName), path)

	fi, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		// The file holds a resolved api_key.
		assert.Equal(t, os.FileMode(0600), fi.Mode().Perm())
	}

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var round map[string]any
	require.NoError(t, yaml.Unmarshal(data, &round))
	requireEq(t, round, "api_key", "0123456789abcdef0123456789abcdef")
	requireEq(t, round, DataPlaneEnabled, true)
}

func TestSetNested(t *testing.T) {
	t.Run("creates intermediate maps", func(t *testing.T) {
		m := map[string]any{}
		setNested(m, "a.b.c", 1)
		assert.Equal(t, map[string]any{"a": map[string]any{"b": map[string]any{"c": 1}}}, m)
	})

	t.Run("overwrites an existing leaf", func(t *testing.T) {
		m := map[string]any{"a": map[string]any{"b": "old"}}
		setNested(m, "a.b", "new")
		assert.Equal(t, map[string]any{"a": map[string]any{"b": "new"}}, m)
	})

	t.Run("replaces a scalar shadowed by a section", func(t *testing.T) {
		m := map[string]any{"a": "scalar"}
		setNested(m, "a.b", 1)
		assert.Equal(t, map[string]any{"a": map[string]any{"b": 1}}, m)
	})

	t.Run("single segment key", func(t *testing.T) {
		m := map[string]any{}
		setNested(m, "a", 1)
		assert.Equal(t, map[string]any{"a": 1}, m)
	})
}

// TestSanitizedEnvStripsDDVars is load-bearing: ADP layers environment variables over its
// config file, so an inherited DD_DOGSTATSD_PORT would make the preflight process bind the
// real DogStatsD endpoint and steal traffic from the Core Agent.
func TestSanitizedEnvStripsDDVars(t *testing.T) {
	got := sanitizedEnv([]string{
		"PATH=/usr/bin",
		"DD_API_KEY=secret",
		"DD_DOGSTATSD_PORT=8125",
		"DD_DOGSTATSD_SOCKET=/var/run/datadog/dsd.socket",
		// Windows environment lookups are case-insensitive, so a mixed-case variant still
		// reaches ADP as the real override and must be stripped too.
		"dd_dogstatsd_port=8125",
		"Dd_Dogstatsd_Socket=/var/run/datadog/dsd.socket",
		"dD_aPi_KeY=secret",
		"HOME=/root",
		// Not DD_-prefixed, so it stays.
		"DDOG_NOT_PREFIXED=keep",
		"ADD_TO_PATH=keep",
	})
	assert.Equal(t, []string{
		"PATH=/usr/bin", "HOME=/root", "DDOG_NOT_PREFIXED=keep", "ADD_TO_PATH=keep",
	}, got)
}

func TestSanitizedEnvEdgeCases(t *testing.T) {
	// Entries shorter than the prefix must not panic on the slice.
	assert.Equal(t, []string{"A", "", "DD"}, sanitizedEnv([]string{"A", "", "DD", "DD_X=1"}))
}
