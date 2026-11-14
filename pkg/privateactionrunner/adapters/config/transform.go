// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"crypto/ecdsa"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	statsdcomp "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/actions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/gobwas/glob"
	"k8s.io/apimachinery/pkg/util/sets"
)

// FromDDConfig builds the runner Config from the Agent config. The metrics client
// is supplied by the caller (the standalone runner builds one with NewMetricsClient;
// the Cluster Agent passes an in-process adapter; callers that emit no metrics, such
// as the identity-rotation commands, may pass nil). A nil client defaults to no-op.
func FromDDConfig(config config.Component, metricsClient statsd.ClientInterface) (*Config, error) {
	if metricsClient == nil {
		metricsClient = &statsd.NoOpClient{}
	}
	ddSite := par.Site(config)
	apiBaseURL, err := par.APIBaseURL(config, ddSite)
	if err != nil {
		return nil, err
	}
	if par.ShouldWarnAboutRCOverride(config) {
		log.Warn("PAR is enabled with remote_configuration.rc_dd_url set but private_action_runner.par_dd_url unset; the RC override does not change PAR routing. Set private_action_runner.par_dd_url explicitly if PAR requires custom routing.")
	}
	encodedPrivateKey := config.GetString(par.PrivateKey)
	urn := config.GetString(par.URN)

	var privateKey *ecdsa.PrivateKey
	if encodedPrivateKey != "" {
		jwk, err := util.Base64ToJWK(encodedPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decode %s: %w", par.PrivateKey, err)
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
	if v := config.GetInt32(par.TaskTimeoutSeconds); v != 0 {
		taskTimeoutSeconds = &v
	}

	httpTimeout := defaultHTTPClientTimeout
	if v := config.GetInt32(par.HTTPTimeoutSeconds); v != 0 {
		httpTimeout = time.Duration(v) * time.Second
	}
	agentHTTPClient := &http.Client{
		Transport: httputils.CreateHTTPTransport(config),
	}

	return &Config{
		MaxBackoff:                         maxBackoff,
		MinBackoff:                         minBackoff,
		MaxAttempts:                        maxAttempts,
		WaitBeforeRetry:                    waitBeforeRetry,
		LoopInterval:                       loopInterval,
		OpmsRequestTimeout:                 opmsRequestTimeout,
		RunnerPoolSize:                     config.GetInt32(par.TaskConcurrency),
		HealthCheckInterval:                healthCheckInterval,
		HttpServerReadTimeout:              defaultHTTPServerReadTimeout,
		HttpServerWriteTimeout:             defaultHTTPServerWriteTimeout,
		HTTPTimeout:                        httpTimeout,
		TaskTimeoutSeconds:                 taskTimeoutSeconds,
		RunnerAccessTokenHeader:            runnerAccessTokenHeader,
		RunnerAccessTokenIdHeader:          runnerAccessTokenIDHeader,
		Port:                               defaultPort,
		JWTRefreshInterval:                 defaultJwtRefreshInterval,
		HealthCheckEndpoint:                defaultHealthCheckEndpoint,
		HeartbeatInterval:                  heartbeatInterval,
		Version:                            version.AgentVersion,
		MetricsClient:                      metricsClient,
		AgentHTTPClient:                    agentHTTPClient,
		ActionsAllowlist:                   makeActionsAllowlist(config),
		Allowlist:                          config.GetStringSlice(par.HTTPAllowlist),
		DomainAllowlist:                    domainAllowlist(config),
		AllowIMDSEndpoint:                  config.GetBool(par.HTTPAllowIMDSEndpoint),
		KubernetesAllowedCustomResources:   kubernetesAllowedCustomResources(config),
		RShellAllowedPaths:                 rshellAllowedPaths(config),
		RShellAllowedCommands:              rshellAllowedCommands(config),
		RShellAllowedSystemServices:        rshellAllowedSystemServices(config),
		RShellDisableDetailedTelemetry:     config.GetBool(par.RestrictedShellDisableDetailedTelemetry),
		RShellPrivilegedEnabled:            config.GetBool(par.RestrictedShellPrivilegedEnabled),
		RShellPrivilegedSocket:             config.GetString(par.RestrictedShellPrivilegedSocket),
		RShellPrivilegedElevatableCommands: rshellElevatableCommands(config),
		RShellAllowedCommandsConfigured:    config.IsConfigured(par.RestrictedShellAllowedCommands),
		RShellAllowedPathsConfigured:       config.IsConfigured(par.RestrictedShellAllowedPaths),
		OpmsExtraHeaders:                   config.GetStringMapString(par.OPMSExtraHeaders),
		APIBaseURL:                         apiBaseURL,
		Modes:                              []modes.Mode{modes.ModePull},
		OrgId:                              orgID,
		PrivateKey:                         privateKey,
		RunnerId:                           runnerID,
		Urn:                                urn,
		DatadogSite:                        ddSite,
	}, nil
}

// kubernetesAllowedCustomResources preserves nil for an unset allowlist so custom
// resource actions can distinguish compatibility mode from an explicit
// empty-list deny-all policy.
func kubernetesAllowedCustomResources(config config.Component) []string {
	if !config.IsConfigured(par.KubernetesAllowedCustomResources) {
		return nil
	}
	return config.GetStringSlice(par.KubernetesAllowedCustomResources)
}

// domainAllowlist returns nil when domain_allowlist is unset so every domain stays
// allowed; an explicit list, even empty, allows only matching domains.
func domainAllowlist(config config.Component) []string {
	if !config.IsConfigured(par.DomainAllowlist) {
		return nil
	}
	// Env values are split on commas without trimming, so "a, b" yields " b".
	patterns := []string{}
	for _, pattern := range config.GetStringSlice(par.DomainAllowlist) {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if _, err := glob.Compile(strings.ToLower(pattern)); err != nil {
			log.Warnf("%s entry %q is not a valid glob pattern and only matches exactly: %v", par.DomainAllowlist, pattern, err)
		}
		patterns = append(patterns, pattern)
	}
	return patterns
}

func makeActionsAllowlist(config config.Component) ActionsAllowlist {
	actionFqns := config.GetStringSlice(par.ActionsAllowlist)

	if config.GetBool(par.DefaultActionsEnabled) {
		if flavor.GetFlavor() == flavor.ClusterAgent {
			actionFqns = append(actionFqns, DefaultClusterAgentActionFQNs...)
		} else {
			actionFqns = append(actionFqns, DefaultActionFQNs...)
		}
	}

	// When the kubeactions subsystem is enabled and its Private Action Runner
	// delivery path is on, auto-allow its bundle so the kubernetes-actions
	// backend (dispatching via wf-actions-server) works without operators having
	// to set actions_allowlist manually.
	if config.GetBool("kubeactions.enabled") && config.GetBool("kubeactions.private_action_runner.enabled") {
		actionFqns = append(actionFqns, KubeActionsActionFQNs...)
	}

	return NewActionsAllowlist(actionFqns)
}

// NewActionsAllowlist parses action FQNs and adds inherited actions without adding defaults.
func NewActionsAllowlist(actionFqns []string) ActionsAllowlist {
	allowlist := make(ActionsAllowlist)
	for _, fqn := range actionFqns {
		bundleName, actionName := actions.SplitFQN(fqn)
		previous, ok := allowlist[bundleName]
		if !ok {
			previous = sets.New[string]()
		}
		allowlist[bundleName] = previous.Insert(actionName)
	}

	for bundleID, actionsSet := range GetBundleInheritedAllowedActions(allowlist) {
		allowlist[bundleID] = allowlist[bundleID].Union(actionsSet)
	}

	return allowlist
}

// rshellAllowedCommands returns the operator-configured rshell command allowlist.
//
// The default value is a wildcard ["rshell:*"] created to match all commands in the rshell namespace.
// See pkg/config/schema/yaml/private_action_runner.yaml for more details.
//
// If the wildcard "rshell:*" is present, the operator-configured list acts as an ALLOW ALL:
// only the backend will be used to filter the commands.
//
// If the wildcard "rshell:*" is not present, the operator-configured list is used to filter the commands.
// For a command to be executed by rshell, it needs to be present in both the operator-configured list
// AND the backend's allowed commands list. (intersection operation)
func rshellAllowedCommands(config config.Component) []string {
	commands := config.GetStringSlice(par.RestrictedShellAllowedCommands)
	warnUnnamespacedCommands(par.RestrictedShellAllowedCommands, commands)
	return commands
}

// Nil means unset; a configured empty map is the explicit deny-all policy.
func rshellAllowedSystemServices(config config.Component) map[string][]string {
	if !config.IsConfigured(par.RestrictedShellAllowedSystemServices) {
		return nil
	}
	return config.GetStringMapStringSlice(par.RestrictedShellAllowedSystemServices)
}

func rshellElevatableCommands(config config.Component) []string {
	if !config.IsConfigured(par.RestrictedShellPrivilegedElevatableCommands) {
		return nil
	}
	commands := config.GetStringSlice(par.RestrictedShellPrivilegedElevatableCommands)
	warnUnnamespacedCommands(par.RestrictedShellPrivilegedElevatableCommands, commands)
	return commands
}

func warnUnnamespacedCommands(key string, commands []string) {
	for _, c := range commands {
		if !strings.HasPrefix(c, RshellCommandNamespacePrefix) {
			log.Warnf(
				"%s entry %q is missing the %q prefix and will never match a backend command; use %q instead",
				key,
				c,
				RshellCommandNamespacePrefix,
				RshellCommandNamespacePrefix+c,
			)
		}
	}
}

// rshellAllowedPaths returns the operator-configured rshell path allowlist.
//
// The default value is ["/"] matching all paths.
// See pkg/config/schema/yaml/private_action_runner.yaml for more details.
//
// The operator-configured list is used to filter the paths.
// For a path to be accessible by rshell, it needs to be present in both the operator-configured list
// AND the backend's allowed paths list. (intersection operation)
func rshellAllowedPaths(config config.Component) []string {
	paths := config.GetStringSlice(par.RestrictedShellAllowedPaths)
	warnBackslashPaths(paths)
	warnNonDirectoryPaths(paths)
	return paths
}

func warnBackslashPaths(paths []string) {
	for _, p := range paths {
		if strings.ContainsRune(p, '\\') {
			log.Warnf("%s entry %q contains a backslash; only forward-slash paths are supported and this entry will never match a backend rule",
				par.RestrictedShellAllowedPaths, p)
		}
	}
}

func warnNonDirectoryPaths(paths []string) {
	for _, p := range paths {
		info, err := os.Stat(p)
		if err == nil && !info.IsDir() {
			log.Warnf("%s entry %q is not a directory; rshell's sandbox only accepts directory entries and will drop this entry at runtime. Use the containing directory instead.",
				par.RestrictedShellAllowedPaths, p)
		}
	}
}

func GetBundleInheritedAllowedActions(actionsAllowlist map[string]sets.Set[string]) map[string]sets.Set[string] {
	result := make(map[string]sets.Set[string])

	for _, inheritedAction := range BundleInheritedAllowedActions {
		actionBundleID, actionName := actions.SplitFQN(inheritedAction.ActionFQN)
		actionBundleID = strings.ToLower(actionBundleID)
		prefix := strings.ToLower(inheritedAction.ExpectedPrefix)

		matched := false
		for bundleID, actionsSet := range actionsAllowlist {
			if actionsSet.Len() > 0 && (bundleID == prefix || strings.HasPrefix(bundleID, prefix+".")) {
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

// NewMetricsClient builds a DogStatsD client from the Agent's configured
// host/port endpoint.
func NewMetricsClient(config config.Component, statsdComp statsdcomp.Component) (statsd.ClientInterface, error) {
	port := config.GetInt("dogstatsd_port")
	client, err := statsdComp.CreateForHostPort(configutils.GetBindHost(config), port)
	if err != nil {
		return &statsd.NoOpClient{}, fmt.Errorf("failed to create DogStatsD client: %w", err)
	}
	return client, nil
}
