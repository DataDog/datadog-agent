// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/actions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-go/v5/statsd"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	maxBackoff                   = 3 * time.Minute
	minBackoff                   = 1 * time.Second
	maxAttempts                  = 20
	waitBeforeRetry              = 5 * time.Minute
	loopInterval                 = 1 * time.Second
	opmsRequestTimeout           = 30_000
	runnerPoolSize               = 1
	defaultHealthCheckEndpoint   = "/healthz"
	healthCheckInterval          = 30_000
	defaultHTTPServerReadTimeout = 10_000
	// defaultHTTPServerWriteTimeout defines how long a request is allowed to run for after the HTTP connection is established. If actions are timing out often, `httpServerWriteTimeout` can be adjusted in config.yaml to override this value. See the Golang docs under `WriteTimeout` for more information about how the server uses this value - https://pkg.go.dev/net/http#Server
	defaultHTTPServerWriteTimeout = 60_000
	runnerAccessTokenHeader       = "X-Datadog-Apps-On-Prem-Runner-Access-Token"
	runnerAccessTokenIDHeader     = "X-Datadog-Apps-On-Prem-Runner-Access-Token-ID"
	defaultPort                   = 9016
	defaultJwtRefreshInterval     = 15 * time.Second
	heartbeatInterval             = 20 * time.Second
)

func FromDDConfig(config config.Component) (*Config, error) {
	ddSite := config.GetString("site")
	encodedPrivateKey := config.GetString("privateactionrunner.private_key")
	urn := config.GetString("privateactionrunner.urn")

	if encodedPrivateKey == "" {
		return nil, errors.New("private action runner not configured: either run enrollment or provide privateactionrunner.private_key")
	}
	privateKey, err := util.Base64ToJWK(encodedPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode privateactionrunner.private_key: %w", err)
	}

	if urn == "" {
		return nil, errors.New("private action runner not configured: URN is required")
	}

	orgID, runnerID, err := parseURN(urn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URN: %w", err)
	}

	actionsAllowlist := prepareAllowList(config.GetStringSlice("privateactionrunner.actions_allowlist"))

	return &Config{
		MaxBackoff:                maxBackoff,
		MinBackoff:                minBackoff,
		MaxAttempts:               maxAttempts,
		WaitBeforeRetry:           waitBeforeRetry,
		LoopInterval:              loopInterval,
		OpmsRequestTimeout:        opmsRequestTimeout,
		RunnerPoolSize:            runnerPoolSize,
		HealthCheckInterval:       healthCheckInterval,
		HttpServerReadTimeout:     defaultHTTPServerReadTimeout,
		HttpServerWriteTimeout:    defaultHTTPServerWriteTimeout,
		RunnerAccessTokenHeader:   runnerAccessTokenHeader,
		RunnerAccessTokenIdHeader: runnerAccessTokenIDHeader,
		Port:                      defaultPort,
		JWTRefreshInterval:        defaultJwtRefreshInterval,
		HealthCheckEndpoint:       defaultHealthCheckEndpoint,
		HeartbeatInterval:         heartbeatInterval,
		Version:                   version.AgentVersion,
		MetricsClient:             &statsd.NoOpClient{},
		ActionsAllowlist:          actionsAllowlist,
		Allowlist:                 strings.Split(config.GetString("privateactionrunner.allowlist"), ","),
		AllowIMDSEndpoint:         config.GetBool("privateactionrunner.allow_imds_endpoint"),
		DDHost:                    strings.Join([]string{"api", ddSite}, "."),
		Modes:                     []modes.Mode{modes.ModePull},
		OrgId:                     orgID,
		PrivateKey:                privateKey.Key.(*ecdsa.PrivateKey),
		RunnerId:                  runnerID,
		Urn:                       urn,
		DatadogSite:               ddSite,
	}, nil
}

func prepareAllowList(fqns []string) map[string]sets.Set[string] {
	allowList := make(map[string]sets.Set[string])
	for _, fqn := range fqns {
		bundleId, action := actions.SplitFQN(fqn)
		if allowList[bundleId] == nil {
			allowList[bundleId] = sets.New[string]()
		}
		allowList[bundleId].Insert(action)
	}
	return allowList
}

// parseURN parses a URN in the format urn:dd:apps:on-prem-runner:{region}:{org_id}:{runner_id}
// and returns the org_id and runner_id
func parseURN(urn string) (int64, string, error) {
	parts := strings.Split(urn, ":")
	if len(parts) != 7 {
		return 0, "", fmt.Errorf("invalid URN format: expected 6 parts separated by ':', got %d", len(parts))
	}

	if parts[0] != "urn" || parts[1] != "dd" || parts[2] != "apps" || parts[3] != "on-prem-runner" {
		return 0, "", fmt.Errorf("invalid URN format: expected 'urn:dd:apps:on-prem-runner', got '%s:%s:%s:%s'", parts[0], parts[1], parts[2], parts[3])
	}

	orgID, err := strconv.ParseInt(parts[5], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid org_id in URN: %w", err)
	}

	runnerID := parts[6]
	if runnerID == "" {
		return 0, "", errors.New("runner_id cannot be empty in URN")
	}

	return orgID, runnerID, nil
}
