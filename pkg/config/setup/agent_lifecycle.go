// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"

const (
	// ExperimentalNodeAgentRolloutEnabled enables the experimental prepared/active Agent lifecycle.
	ExperimentalNodeAgentRolloutEnabled = "experimental.node_agent_rollout.enabled"
	// ExperimentalNodeAgentRolloutLockPath is the node-local ownership lock used by one Agent process. Shared configuration must include {component}.
	ExperimentalNodeAgentRolloutLockPath = "experimental.node_agent_rollout.lock_path"
	// ExperimentalNodeAgentRolloutStatePath reports the local lifecycle state of one Agent process. Shared configuration must include {component}.
	ExperimentalNodeAgentRolloutStatePath = "experimental.node_agent_rollout.state_path"
)

func setupExperimentalNodeAgentRollout(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault(ExperimentalNodeAgentRolloutEnabled, false)
	config.BindEnvAndSetDefault(ExperimentalNodeAgentRolloutLockPath, "")
	config.BindEnvAndSetDefault(ExperimentalNodeAgentRolloutStatePath, "")
}
