// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import "time"

const (
	maxBackoff                   = 3 * time.Minute
	minBackoff                   = 1 * time.Second
	maxAttempts                  = 20
	waitBeforeRetry              = 5 * time.Minute
	loopInterval                 = 1 * time.Second
	opmsRequestTimeout           = 30_000
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
	defaultHTTPClientTimeout      = 30 * time.Second
)

// BundleInheritedAllowedActions is a list of actions that are automatically allowed
// if at least one other action from the same bundle is allowed
var BundleInheritedAllowedActions = []string{
	"com.datadoghq.gitlab.users.testConnection",
	"com.datadoghq.kubernetes.core.testConnection",
	"com.datadoghq.script.testConnection",
	"com.datadoghq.script.enrichScript",
	"com.datadoghq.ddagent.testConnection",
}
