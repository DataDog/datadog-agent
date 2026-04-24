// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/actions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-go/v5/statsd"
	"k8s.io/apimachinery/pkg/util/sets"
)

// rshellCommandNamespace is the prefix the backend stamps onto every command
// in its allow-list. Operator entries in datadog.yaml must use the same form
// to intersect; otherwise they silently fail to match.
const rshellCommandNamespace = "rshell:"

func FromDDConfig(config config.Component) (*Config, error) {
	mainEndpoint := configutils.GetMainEndpoint(config, "https://api.", "dd_url")
	ddHost := getDatadogHost(mainEndpoint)
	ddSite := configutils.ExtractSiteFromURL(mainEndpoint)
	encodedPrivateKey := config.GetString(setup.PARPrivateKey)
	urn := config.GetString(setup.PARUrn)

	var privateKey *ecdsa.PrivateKey
	if encodedPrivateKey != "" {
		jwk, err := util.Base64ToJWK(encodedPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decode %s: %w", setup.PARPrivateKey, err)
		}
		privateKey = jwk.Key.(*ecdsa.PrivateKey)
	}

	var orgID int64
	var runnerID string
	// allow empty urn for self-enrollment
	if urn != "" {
		urnParts, err := util.ParseRunnerURN(urn)
		if err != nil {
			return nil, fmt.Errorf("failed to parse URN: %w", err)
		}
		orgID = urnParts.OrgID
		runnerID = urnParts.RunnerID
	}

	var taskTimeoutSeconds *int32
	if v := config.GetInt32(setup.PARTaskTimeoutSeconds); v != 0 {
		taskTimeoutSeconds = &v
	}

	httpTimeout := defaultHTTPClientTimeout
	if v := config.GetInt32(setup.PARHttpTimeoutSeconds); v != 0 {
		httpTimeout = time.Duration(v) * time.Second
	}

	return &Config{
		MaxBackoff:                maxBackoff,
		MinBackoff:                minBackoff,
		MaxAttempts:               maxAttempts,
		WaitBeforeRetry:           waitBeforeRetry,
		LoopInterval:              loopInterval,
		OpmsRequestTimeout:        opmsRequestTimeout,
		RunnerPoolSize:            config.GetInt32(setup.PARTaskConcurrency),
		HealthCheckInterval:       healthCheckInterval,
		HttpServerReadTimeout:     defaultHTTPServerReadTimeout,
		HttpServerWriteTimeout:    defaultHTTPServerWriteTimeout,
		HTTPTimeout:               httpTimeout,
		TaskTimeoutSeconds:        taskTimeoutSeconds,
		RunnerAccessTokenHeader:   runnerAccessTokenHeader,
		RunnerAccessTokenIdHeader: runnerAccessTokenIDHeader,
		Port:                      defaultPort,
		JWTRefreshInterval:        defaultJwtRefreshInterval,
		HealthCheckEndpoint:       defaultHealthCheckEndpoint,
		HeartbeatInterval:         heartbeatInterval,
		Version:                   version.AgentVersion,
		MetricsClient:             &statsd.NoOpClient{},
		ActionsAllowlist:          makeActionsAllowlist(config),
		Allowlist:                 config.GetStringSlice(setup.PARHttpAllowlist),
		AllowIMDSEndpoint:         config.GetBool(setup.PARHttpAllowImdsEndpoint),
		RShellAllowedPaths:        rshellAllowedPaths(config),
		RShellAllowedCommands:     rshellAllowedCommands(config),
		DDHost:                    ddHost,
		DDApiHost:                 "api." + ddSite,
		Modes:                     []modes.Mode{modes.ModePull},
		OrgId:                     orgID,
		PrivateKey:                privateKey,
		RunnerId:                  runnerID,
		Urn:                       urn,
		DatadogSite:               ddSite,
	}, nil
}

func makeActionsAllowlist(config config.Component) map[string]sets.Set[string] {
	allowlist := make(map[string]sets.Set[string])
	actionFqns := config.GetStringSlice(setup.PARActionsAllowlist)

	if config.GetBool(setup.PARDefaultActionsEnabled) {
		if flavor.GetFlavor() == flavor.ClusterAgent {
			actionFqns = append(actionFqns, DefaultClusterAgentActionFQNs...)
		} else {
			actionFqns = append(actionFqns, DefaultActionFQNs...)
		}
	}

	for _, fqn := range actionFqns {
		bundleName, actionName := actions.SplitFQN(fqn)
		previous, ok := allowlist[bundleName]
		if !ok {
			previous = sets.New[string]()
		}
		allowlist[bundleName] = previous.Insert(actionName)
	}

	bundleInheritedActions := GetBundleInheritedAllowedActions(allowlist)
	for bundleID, actionsSet := range bundleInheritedActions {
		allowlist[bundleID] = allowlist[bundleID].Union(actionsSet)
	}

	return allowlist
}

// rshellAllowedCommands returns the operator-configured rshell command
// allowlist, or nil when the operator did not configure one. Nil signals
// "pass-through" so the handler forwards the backend list unchanged; a
// non-nil empty slice is the explicit "block everything" kill-switch.
//
// Entries are expected to be in the backend's namespaced form
// ("rshell:<name>"). Operators spelling commands without the prefix are
// warned at load time so the silent no-match failure mode is observable.
func rshellAllowedCommands(config config.Component) []string {
	commands := configuredStringSliceOrNil(config, setup.PARRestrictedShellAllowedCommands)
	warnUnnamespacedCommands(commands)
	return commands
}

// warnUnnamespacedCommands emits a log warning for each entry missing the
// "rshell:" prefix. These entries will never match a backend command, so an
// operator who writes `allowed_commands: [cat]` instead of
// `allowed_commands: [rshell:cat]` would otherwise get silent kill-switch
// behavior with no feedback.
func warnUnnamespacedCommands(commands []string) {
	for _, c := range commands {
		if !strings.HasPrefix(c, rshellCommandNamespace) {
			log.Warnf("%s entry %q is missing the %q prefix and will never match a backend command; use %q instead",
				setup.PARRestrictedShellAllowedCommands, c, rshellCommandNamespace, rshellCommandNamespace+c)
		}
	}
}

// rshellAllowedPaths mirrors rshellAllowedCommands for the filesystem
// allowlist. Nil means "operator unset" — the handler will pass the backend
// list through unchanged rather than tightening to an empty intersection.
//
// Two advisory warnings fire at load time so misconfiguration is
// observable to the operator before any task runs. Both leave the entries
// in place and let the intersection / rshell's own sandbox do the final
// filtering:
//
//   - backslash entries fail to match the Linux-form backend list at the
//     intersection layer;
//   - non-directory entries are silently skipped by rshell's os.Root-based
//     sandbox at runner creation. The operator-visible symptom is
//     permission-denied with no explanation, so we surface the reason in
//     the agent log once at load time.
func rshellAllowedPaths(config config.Component) []string {
	paths := configuredStringSliceOrNil(config, setup.PARRestrictedShellAllowedPaths)
	warnBackslashPaths(paths)
	warnNonDirectoryPaths(paths)
	return paths
}

// warnBackslashPaths emits a log warning for each entry containing a
// backslash. The operator-side allow-list is defined as forward-slash only;
// Windows-native paths will never match the backend's Linux-style entries
// and would otherwise fail silently.
func warnBackslashPaths(paths []string) {
	for _, p := range paths {
		if strings.ContainsRune(p, '\\') {
			log.Warnf("%s entry %q contains a backslash; only forward-slash paths are supported and this entry will never match a backend path",
				setup.PARRestrictedShellAllowedPaths, p)
		}
	}
}

// warnNonDirectoryPaths emits a log warning for entries that exist on disk
// but are not directories. rshell's sandbox is built on os.Root, which
// only accepts directory handles, so file entries get silently dropped at
// runner creation and every open inside the task returns permission-
// denied. Entries that do not exist at load time are not warned about:
// rshell's own "path not found" warning at task time covers that case.
func warnNonDirectoryPaths(paths []string) {
	for _, p := range paths {
		info, err := os.Stat(p)
		if err == nil && !info.IsDir() {
			log.Warnf("%s entry %q is not a directory; rshell's sandbox only accepts directory entries and will drop this entry at runtime. Use the containing directory instead.",
				setup.PARRestrictedShellAllowedPaths, p)
		}
	}
}

// configuredStringSliceOrNil returns the configured value only when the key
// was explicitly set by the user (YAML file, env, or SetWithoutSource); it
// returns nil when the key is still at its default.
//
// GetStringSlice returns a nil slice for an explicit YAML empty list
// (`key: []`), which is indistinguishable at the slice level from "unset".
// IsConfigured is the authoritative signal for "user touched this key", so we
// gate on that and then normalize nil → non-nil empty to preserve the
// "operator explicitly allowed nothing" semantics downstream.
func configuredStringSliceOrNil(config config.Component, key string) []string {
	if !config.IsConfigured(key) {
		return nil
	}
	v := config.GetStringSlice(key)
	if v == nil {
		return []string{}
	}
	return v
}

// getDatadogHost extracts and normalizes the Datadog host from the main endpoint.
// It removes the "https://" prefix and any trailing "." from the endpoint URL.
func getDatadogHost(endpoint string) string {
	host := strings.TrimSuffix(endpoint, ".")
	host = strings.TrimPrefix(host, "https://")
	return host
}

func GetBundleInheritedAllowedActions(actionsAllowlist map[string]sets.Set[string]) map[string]sets.Set[string] {
	result := make(map[string]sets.Set[string])

	for _, inheritedAction := range BundleInheritedAllowedActions {
		actionBundleID, actionName := actions.SplitFQN(inheritedAction.ActionFQN)
		actionBundleID = strings.ToLower(actionBundleID)
		prefix := strings.ToLower(inheritedAction.ExpectedPrefix)

		matched := false
		for bundleID, actionsSet := range actionsAllowlist {
			if actionsSet.Len() > 0 && strings.HasPrefix(bundleID, prefix) {
				matched = true
				break
			}
		}

		if !matched {
			continue
		}

		if _, exists := result[actionBundleID]; !exists {
			result[actionBundleID] = sets.New[string]()
		}
		result[actionBundleID].Insert(actionName)
	}

	return result
}
