// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package preflightmodeimpl

import (
	"fmt"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// isEligible reports whether the Agent Data Plane should be run in preflight mode, along with a human-readable reason
// when it should not be. The reason names settings, never their values.
//
// Preflight mode is a pre-flight check: it starts ADP for a short window, in an isolated fashion, so that
// environment-specific startup problems can be reported back to Datadog prior to ADP being released as GA. It therefore
// only runs when `data_plane.enabled` is `false` _and_ `false` is coming from the default value.
//
// We additionally evaluate a preflight mode-specific setting -- `data_plane.preflight_mode` -- which provides a means
// to disable preflight mode explicitly if preflight mode itself manages to surface any customer-visible errors.
//
// Overall, this ensures that we don't run ADP at all when the operator explicitly indicates that ADP should be disabled
// (`data_plane.enabled: false`) or when it's enabled, since we'd be running a second ADP process unnecessarily.
//
// Finally, we skip the pre-flight whenever secrets are in the picture at all -- see secretsInUse.
//
// Note that the platform gate in pkg/config/setup locks `data_plane.enabled` to `false` via SourceAgentRuntime where
// ADP cannot run at all -- unsupported platforms, and Windows without process_manager.enabled -- which makes this
// return false there too. That is what we want: there is nothing to pre-flight in an environment ADP could never run
// in.
func isEligible(config pkgconfigmodel.Reader) (bool, string) {
	if !config.GetBool(DataPlanePreflightMode) {
		return false, DataPlanePreflightMode + " is false"
	}

	dataPlaneEnabled := config.GetBool(DataPlaneEnabled)
	dataPlaneEnabledSource := config.GetSource(DataPlaneEnabled)
	if dataPlaneEnabled || dataPlaneEnabledSource != pkgconfigmodel.SourceDefault {
		return false, fmt.Sprintf("%s is %t and was set by %q", DataPlaneEnabled, dataPlaneEnabled, dataPlaneEnabledSource)
	}

	if inUse, reason := secretsInUse(config); inUse {
		return false, reason
	}

	return true, ""
}

// Settings that turn the secret resolver on. These mirror the gate in pkg/config/setup that decides whether to run the
// resolver over the Agent's configuration at all.
const (
	secretBackendCommand = "secret_backend_command"
	secretBackendType    = "secret_backend_type"
	multiSecretBackends  = "multi_secret_backends"
)

// secretsInUse reports whether the Agent's configuration involves secrets at all, along with a human-readable reason
// when it does.
//
// This is a gate on preflight mode because the pre-flight writes the Agent's entire resolved configuration to disk for
// ADP to read (see writePreflightConfig). Anything the secret resolver pulled into memory would therefore be
// materialized in plaintext in a file. The working directory is locked down to the Agent's own account and removed when
// the run finishes, but a value the operator deliberately kept out of any file on the host should not be written to one
// for the sake of a pre-flight check that only exists until ADP goes GA. So when secrets are involved we skip the
// pre-flight outright rather than try to redact a config we do not fully understand -- ADP consumes settings the Core
// Agent's schema does not describe.
//
// Both halves of the check matter. The backend settings catch a resolver that is configured but has not put anything
// into the Agent config yet: handles that only appear in integration configs, or a refresh interval whose first refresh
// is still to come. The secrets layer catches values that have already been resolved, including from a backend this
// does not know to look for.
func secretsInUse(config pkgconfigmodel.Reader) (bool, string) {
	for _, setting := range []string{secretBackendCommand, secretBackendType} {
		if config.GetString(setting) != "" {
			return true, setting + " is set"
		}
	}
	if len(config.GetStringMap(multiSecretBackends)) > 0 {
		return true, multiSecretBackends + " is set"
	}
	if found, setting := getFirstSecretSetting(config); found {
		return true, fmt.Sprintf("detected setting (%s) in secrets source layer", setting)
	}
	return false, ""
}

// getFirstSecretSetting returns the name of a setting carrying a value in the config's secrets layer, if there is one.
//
// The secrets layer has no dedicated accessor (it's intentionally omitted from inclusion in typical "get all settings"
// methods) so we explicitly iterate over all keys in the configuration and then check to see if any of them have a
// value present in the secrets layer.
func getFirstSecretSetting(config pkgconfigmodel.Reader) (bool, string) {
	for _, setting := range config.AllKeysLowercased() {
		for _, value := range config.GetAllSources(setting) {
			if value.Source == pkgconfigmodel.SourceSecret && value.Value != nil {
				return true, setting
			}
		}
	}
	return false, ""
}
