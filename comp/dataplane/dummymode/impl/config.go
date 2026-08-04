// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package dummymodeimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v3"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	dummyConfigFileName = "datadog.yaml"

	DataPlaneEnabled     = "data_plane.enabled"
	DataPlaneDummyMode   = "data_plane.dummy_mode"
	DataPlaneStopTimeout = "data_plane.stop_timeout"
)

// dummyModeDataPlaneOnlyOverrides holds settings ADP understands but the Core Agent does
// not, so they cannot be checked against the Agent's config schema.
//
// Keep this map as small as possible. A typo here is silent in both directions: the Agent
// cannot validate the key, and ADP ignores what it does not recognise, so the setting
// simply does not take effect and the pre-flight quietly stops being isolated.
//
// standalone_mode is verified indirectly: ADP logs "Running in standalone mode." in
// response to it, and logscan_test.go pins that record as realWarnStandalone. If the key
// were wrong, that warning would disappear.
var dummyModeDataPlaneOnlyOverrides = map[string]any{
	// Do not contact the Core Agent at all, for either registration or configuration.
	"data_plane.standalone_mode": true,
}

// dummyModeGlobalOverrides lists the Core Agent settings that should be overridden in ADP's
// dummy mode configuration regardless of OS/architecture.
//
// Every key here must be a real Agent setting; TestOverrideKeysAreKnown enforces that.
// Overriding a key that does not exist is silent and actively harmful: it writes something
// ADP ignores while the setting it was meant to replace passes through from AllSettings.
var dummyModeGlobalOverrides = map[string]any{
	// Ensure that ADP is enabled overall, that we're at least running the DogStatsD
	// pipeline, but that OTLP is disabled.
	//
	// The proxy has its own flag, and it is the dangerous one: the Core Agent acts on
	// data_plane.otlp.proxy.enabled with no data_plane.enabled gate (comp/otelcol/otlp/config.go),
	// moving its own receiver aside because "ADP owns the configured gRPC endpoint". On a
	// host with the proxy enabled but data_plane.enabled unset — which is dummy-eligible —
	// leaving this inherited could put the dummy process on :4317 in front of real customer
	// OTLP traffic for the length of the window.
	DataPlaneEnabled:                true,
	"data_plane.dogstatsd.enabled":  true,
	"data_plane.otlp.enabled":       false,
	"data_plane.otlp.proxy.enabled": false,

	// Keep ADP from contacting the Core Agent to register as a Remote Agent or to fetch
	// configuration. See also dummyModeDataPlaneOnlyOverrides.
	"data_plane.remote_agent_enabled":           false,
	"data_plane.use_new_config_stream_endpoint": false,

	// Configure ADP's various API endpoints to listen on ephemeral ports: we don't
	// want to collide with anything, but we do want to ensure we can properly bind them.
	"data_plane.api_listen_address":        "tcp://127.0.0.1:0",
	"data_plane.secure_api_listen_address": "tcp://127.0.0.1:0",
	"data_plane.telemetry_enabled":         false,

	// Adjust our logging so that we don't log to file, but only console, and via JSON
	// to make it possible to collect all the logs from ADP in a simple way without
	// spilling out to disk.
	"disable_file_logging": true,
	"log_to_console":       true,
	"log_format_json":      true,
	"log_level":            "info",

	// Turn off the forwarder's on-disk retry queue.
	//
	// ADP resolves forwarder_storage_path and, when this setting is non-zero, creates
	// <forwarder_storage_path>/<queue id> at startup and persists transactions there on
	// overflow (lib/saluki-components/src/common/datadog/{retry,io}.rs). Inherited unchanged
	// that path is the Core Agent's own retry tree, so the pre-flight would create directories
	// — and, on overflow, write customer payloads — outside the working directory the run
	// secures and deletes, then leave them behind on every Agent start.
	//
	// This is the gate rather than the path because it is the only one that actually stops the
	// feature. Clearing forwarder_storage_path does not: fix_empty_storage_path falls back to
	// run_path + "transactions_to_retry", landing on the Core Agent's tree anyway, and a path
	// that did end up empty makes with_disk_persistence fail and ADP log at ERROR — which the
	// log scan would then report as findingErrorsInLog, manufacturing a finding on every host
	// that has persistence enabled.
	//
	// Losing coverage of the persistence path is an acceptable trade: it is off by default on
	// both sides, and a pre-flight has nothing to retry anyway. It runs for 90s against a live
	// intake, so the queue only fills if the endpoint is already failing — the case the log
	// scan reports regardless.
	"forwarder_storage_max_size_in_bytes": 0,

	// Configure DogStatsD to avoid listening on UDP or UDS stream, as well as to avoid
	// applying any metric prefixes/namespaces that would mess up our probe metric, which
	// is named specifically to ensure that the metric makes it through to the Datadog
	// backend unchanged.
	//
	// Note the keys are statsd_*, not dogstatsd_* — see comp/dogstatsd/server/impl/server.go.
	// Getting this wrong is silent: an unknown key is simply carried into ADP's config and
	// ignored, while the operator's real namespace passes through and prefixes the probe.
	// TestOverrideKeysAreKnown exists to make that failure impossible.
	"statsd_metric_namespace":           "",
	"statsd_metric_namespace_blacklist": []string{},
	"dogstatsd_port":                    0,
	"dogstatsd_stream_socket":           "",
	"dogstatsd_non_local_traffic":       false,
	"dogstatsd_metrics_stats_enable":    false,
}

// buildDummyConfig returns the configuration ADP should run with during a dummy mode
// pre-flight.
//
// The full Agent configuration as it exists at the time of this call is used as the base,
// and overrides are applied on top of it: this ensures that ADP is configured as close as possible
// to how it would be when running normally, with only the necessary changes to run it in "dummy" mode:
// don't take over DSD, don't run any other pipelines, don't log to disk, etc.
func buildDummyConfig(cfg pkgconfigmodel.Reader, l listener) map[string]any {
	out := cfg.AllSettings()
	if out == nil {
		out = map[string]any{}
	}

	// Set all of the "global" overrides: overrides that apply regardless of OS/architecture.
	for k, v := range dummyModeGlobalOverrides {
		setNested(out, k, v)
	}

	// Settings only ADP knows about, which the Agent's schema cannot validate.
	for k, v := range dummyModeDataPlaneOnlyOverrides {
		setNested(out, k, v)
	}

	// Grab the DogStatsD-specific overrides, which relates to OS/architecture-specific configuration
	// for where DSD will be listening.
	for k, v := range l.configOverrides() {
		setNested(out, k, v)
	}

	return out
}

// writeDummyConfig renders the dummy configuration into workDir and returns its path.
//
// The file holds the Agent's entire resolved configuration. That is every secret, not just
// api_key: AllSettings merges the secrets layer, so secret-backend outputs such as app_key,
// proxy credentials, additional_endpoints keys and integration passwords are all present in
// plaintext. The working directory is removed when the run finishes, and again from stop if
// the run does not unwind in time.
//
// The 0600 below is what restricts the file on Unix. It does nothing on Windows, where the
// mode is not an access control mechanism at all; there the file is covered by the ACL
// secureWorkDir puts on the directory, which is inheritable for exactly this reason.
func writeDummyConfig(cfg pkgconfigmodel.Reader, l listener, workDir string) (string, error) {
	data, err := yaml.Marshal(buildDummyConfig(cfg, l))
	if err != nil {
		return "", fmt.Errorf("could not render the data plane dummy mode config: %w", err)
	}

	path := filepath.Join(workDir, dummyConfigFileName)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("could not write %s: %w", path, err)
	}
	return path, nil
}

// setNested writes a dotted config key into a nested map, creating intermediate maps as
// needed. An intermediate value that is not a map is replaced, which mirrors how the
// config layers resolve a scalar shadowed by a section.
func setNested(m map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	for _, part := range parts[:len(parts)-1] {
		next, ok := m[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[part] = next
		}
		m = next
	}
	m[parts[len(parts)-1]] = value
}
