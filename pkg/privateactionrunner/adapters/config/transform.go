// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"crypto/ecdsa"
	"fmt"
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
// allowlist. Nil = pass-through; non-nil empty = kill-switch.
// Operator entries must be in the backend's namespaced form ("rshell:<name>");
// any other spelling is warned at load time.
func rshellAllowedCommands(config config.Component) []string {
	commands := configuredStringSliceOrNil(config, setup.PARRestrictedShellAllowedCommands)
	warnUnnamespacedCommands(commands)
	return commands
}

// warnUnnamespacedCommands logs a warning per entry missing the "rshell:"
// prefix so the silent no-match failure mode is observable.
func warnUnnamespacedCommands(commands []string) {
	for _, c := range commands {
		if !strings.HasPrefix(c, setup.RShellCommandNamespacePrefix) {
			log.Warnf("%s entry %q is missing the %q prefix and will never match a backend command; use %q instead",
				setup.PARRestrictedShellAllowedCommands, c, setup.RShellCommandNamespacePrefix, setup.RShellCommandNamespacePrefix+c)
		}
	}
}

// rshellAllowedPaths mirrors rshellAllowedCommands for the filesystem
// allowlist.
func rshellAllowedPaths(config config.Component) []string {
	return configuredStringSliceOrNil(config, setup.PARRestrictedShellAllowedPaths)
}

// configuredStringSliceOrNil returns the configured value only when the
// user explicitly set the key. Normalizes the YAML-empty-list edge case,
// where GetStringSlice returns a nil slice indistinguishable from "unset",
// into a non-nil empty slice so the kill-switch is honored.
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
