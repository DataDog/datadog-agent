// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
)

// KubernetesAgent is an agent running in a Kubernetes cluster
type KubernetesAgent struct {
	agent.KubernetesAgentOutput

	// Client cannot be initialized inline as it requires other information to create client
	// Client agentclient.Agent // Do we need to communicate with the Agent, currently tests do not
}
