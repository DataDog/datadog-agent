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

// BundleInheritedAllowedAction represents an action that is automatically allowed
// if at least one other action matching the expected prefix is allowed
type BundleInheritedAllowedAction struct {
	ActionFQN      string
	ExpectedPrefix string
}

// DefaultClusterAgentActionFQNs is a list of action FQNs that are enabled by default
// when the agent runs as a Cluster Agent flavor.
// Users can opt out by setting private_action_runner.default_actions_enabled to false.
var DefaultClusterAgentActionFQNs = []string{
	// k8s apps — Deployments
	"com.datadoghq.kubernetes.apps.listDeployment",
	"com.datadoghq.kubernetes.apps.getDeployment",
	// k8s apps — DaemonSets
	"com.datadoghq.kubernetes.apps.getDaemonSet",
	"com.datadoghq.kubernetes.apps.listDaemonSet",
	// k8s apps — StatefulSets
	"com.datadoghq.kubernetes.apps.getStatefulSet",
	"com.datadoghq.kubernetes.apps.listStatefulSet",
	// k8s core — Pods
	"com.datadoghq.kubernetes.core.getPod",
	"com.datadoghq.kubernetes.core.listPod",
	// k8s core — ConfigMaps
	"com.datadoghq.kubernetes.core.getConfigMap",
	"com.datadoghq.kubernetes.core.listConfigMap",
	// k8s core — Services
	"com.datadoghq.kubernetes.core.getService",
	"com.datadoghq.kubernetes.core.listService",
	// k8s core — Nodes
	"com.datadoghq.kubernetes.core.getNode",
	"com.datadoghq.kubernetes.core.listNode",
	// k8s core — Events (diagnostic context)
	"com.datadoghq.kubernetes.core.listEvent",
	// k8s core — Namespaces
	"com.datadoghq.kubernetes.core.listNamespace",
	// k8s batch — Jobs
	"com.datadoghq.kubernetes.batch.getJob",
	"com.datadoghq.kubernetes.batch.listJob",
	"com.datadoghq.kubernetes.batch.getCronJob",
	"com.datadoghq.kubernetes.batch.listCronJob",
}

// DefaultActionFQNs is a list of action FQNs that are enabled by default
// for non-Cluster-Agent flavors.
// Users can opt out by setting private_action_runner.default_actions_enabled to false.
var DefaultActionFQNs = []string{
	"com.datadoghq.remoteaction.rshell.runCommand",
}

// BundleInheritedAllowedActions is a list of actions that are automatically allowed
// if at least one other action matching their expected prefix is allowed
var BundleInheritedAllowedActions = []BundleInheritedAllowedAction{
	{ActionFQN: "com.datadoghq.gitlab.users.testConnection", ExpectedPrefix: "com.datadoghq.gitlab"},
	{ActionFQN: "com.datadoghq.kubernetes.core.testConnection", ExpectedPrefix: "com.datadoghq.kubernetes"},
	{ActionFQN: "com.datadoghq.script.testConnection", ExpectedPrefix: "com.datadoghq.script"},
	{ActionFQN: "com.datadoghq.script.enrichScript", ExpectedPrefix: "com.datadoghq.script"},
	{ActionFQN: "com.datadoghq.http.testConnection", ExpectedPrefix: "com.datadoghq.http"},
	{ActionFQN: "com.datadoghq.remoteaction.testConnection", ExpectedPrefix: "com.datadoghq.remoteaction"},
}
