// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package preflightmodeimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v3"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	preflightConfigFileName = "datadog.yaml"

	DataPlaneEnabled       = "data_plane.enabled"
	DataPlanePreflightMode = "data_plane.preflight_mode"
	DataPlaneStopTimeout   = "data_plane.stop_timeout"
)

// preflightModeDataPlaneOnlyOverrides holds settings ADP understands but the Core Agent does not, so they cannot be
// checked against the Agent's config schema.
var preflightModeDataPlaneOnlyOverrides = map[string]any{
	// Do not contact the Core Agent at all, for either registration or configuration.
	"data_plane.standalone_mode": true,
}

// preflightModeGlobalOverrides lists the Core Agent settings that should be overridden in ADP's
// preflight mode configuration regardless of OS/architecture.
var preflightModeGlobalOverrides = map[string]any{
	// Ensure that ADP is enabled overall, that we're at least running the DogStatsD pipeline, but that OTLP is
	// disabled.
	DataPlaneEnabled:                true,
	"data_plane.dogstatsd.enabled":  true,
	"data_plane.otlp.enabled":       false,
	"data_plane.otlp.proxy.enabled": false,

	// Keep ADP from contacting the Core Agent to register as a Remote Agent or to fetch configuration. See also
	// preflightModeDataPlaneOnlyOverrides.
	"data_plane.remote_agent_enabled":           false,
	"data_plane.use_new_config_stream_endpoint": false,

	// Configure ADP's various API endpoints to listen on ephemeral ports: we don't want to collide with anything, but
	// we do want to ensure we can properly bind them.
	"data_plane.api_listen_address":        "tcp://127.0.0.1:0",
	"data_plane.secure_api_listen_address": "tcp://127.0.0.1:0",
	"data_plane.telemetry_enabled":         false,

	// Adjust our logging so that we don't log to file, but only console, and via JSON to make it possible to collect
	// all the logs from ADP in a simple way without spilling out to disk.
	"disable_file_logging": true,
	"log_to_console":       true,
	"log_format_json":      true,
	"log_level":            "info",

	// Turn off the forwarder's on-disk retry queue.
	//
	// This ensures we don't drop any retry files on disk or deal with creating subsequent directories, as we don't need
	// to test this feature out in particular and we don't want to risk any collisions with real retry files for the
	// Core Agent itself.
	"forwarder_storage_max_size_in_bytes": 0,

	// Configure DogStatsD to avoid listening on UDP or UDS stream, as well as to avoid applying any metric
	// prefixes/namespaces that would mess up our probe metric, which is named specifically to ensure that the metric
	// makes it through to the Datadog backend unchanged.
	"statsd_metric_namespace":           "",
	"statsd_metric_namespace_blacklist": []string{},
	"dogstatsd_port":                    0,
	"dogstatsd_stream_socket":           "",
	"dogstatsd_non_local_traffic":       false,
	"dogstatsd_metrics_stats_enable":    false,
}

// buildPreflightConfig returns the configuration ADP should run with during a preflight mode
// pre-flight.
//
// The full Agent configuration as it exists at the time of this call is used as the base,
// and overrides are applied on top of it: this ensures that ADP is configured as close as possible
// to how it would be when running normally, with only the necessary changes to run it in "preflight" mode:
// don't take over DSD, don't run any other pipelines, don't log to disk, etc.
func buildPreflightConfig(cfg pkgconfigmodel.Reader, l listener) map[string]any {
	out := cfg.AllSettings()
	if out == nil {
		out = map[string]any{}
	}

	// Set all of the "global" overrides: overrides that apply regardless of OS/architecture.
	for k, v := range preflightModeGlobalOverrides {
		setNested(out, k, v)
	}

	// Settings only ADP knows about, which the Agent's schema cannot validate.
	for k, v := range preflightModeDataPlaneOnlyOverrides {
		setNested(out, k, v)
	}

	// Grab the DogStatsD-specific overrides, which relates to OS/architecture-specific configuration
	// for where DSD will be listening.
	for k, v := range l.configOverrides() {
		setNested(out, k, v)
	}

	return out
}

// writePreflightConfig renders the preflight configuration into workDir and returns its path.
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
func writePreflightConfig(cfg pkgconfigmodel.Reader, l listener, workDir string) (string, error) {
	data, err := yaml.Marshal(buildPreflightConfig(cfg, l))
	if err != nil {
		return "", fmt.Errorf("could not render the data plane preflight mode config: %w", err)
	}

	path := filepath.Join(workDir, preflightConfigFileName)
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
