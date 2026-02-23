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
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-go/v5/statsd"
	"k8s.io/apimachinery/pkg/util/sets"
)

func FromDDConfig(config config.Component) (*Config, error) {
	ddSite := getDatadogSite(config)
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
		DDHost:                    strings.Join([]string{"api", ddSite}, "."),
		DDApiHost:                 strings.Join([]string{"api", ddSite}, "."),
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

func getDatadogSite(config config.Component) string {
	ddSite := ""
	ddURL := config.GetString("dd_url")
	if ddURL != "" {
		extractedSite := configutils.ExtractSiteFromURL(ddURL)
		if extractedSite != "" {
			ddSite = extractedSite
		}
	}
	if ddSite == "" {
		ddSite = config.GetString("site")
	}
	if ddSite == "" {
		ddSite = setup.DefaultSite
	}
	return ddSite
}

func GetBundleInheritedAllowedActions(actionsAllowlist map[string]sets.Set[string]) map[string]sets.Set[string] {
	result := make(map[string]sets.Set[string])

	for _, specialAction := range BundleInheritedAllowedActions {
		specialBundleID, specialActionName := actions.SplitFQN(specialAction)
		specialBundleID = strings.ToLower(specialBundleID)

		actionsSet, ok := actionsAllowlist[specialBundleID]
		if !ok || actionsSet.Len() == 0 {
			continue
		}

		if _, exists := result[specialBundleID]; !exists {
			result[specialBundleID] = sets.New[string]()
		}
		result[specialBundleID].Insert(specialActionName)
	}

	return result
}
