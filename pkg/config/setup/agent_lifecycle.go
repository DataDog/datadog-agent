// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"

const (
	// ExperimentalNodeAgentRolloutEnabled enables the experimental prepared/active Agent lifecycle.
	ExperimentalNodeAgentRolloutEnabled = "experimental.node_agent_rollout.enabled"
	// ExperimentalNodeAgentRolloutPodUID identifies the Pod that must wait for older siblings owned by the same DaemonSet.
	ExperimentalNodeAgentRolloutPodUID = "experimental.node_agent_rollout.pod_uid"
	// ExperimentalNodeAgentRolloutStatePath reports the local lifecycle state of one Agent process. Shared configuration must include {component}.
	ExperimentalNodeAgentRolloutStatePath = "experimental.node_agent_rollout.state_path"
)

func setupExperimentalNodeAgentRollout(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault(ExperimentalNodeAgentRolloutEnabled, false)
	config.BindEnvAndSetDefault(ExperimentalNodeAgentRolloutPodUID, "")
	config.BindEnvAndSetDefault(ExperimentalNodeAgentRolloutStatePath, "")
}
