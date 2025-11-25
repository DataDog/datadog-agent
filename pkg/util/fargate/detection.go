// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fargate

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// IsSidecar returns whether the Agent is running in a sidecar deployment mode.
// This includes:
// - ECS Fargate (always sidecar)
// - EKS Fargate
// - ECS Managed Instances in sidecar mode (explicitly configured via ecs_deployment_mode: sidecar)
// These environments all share the characteristic that the agent runs as a sidecar container
// and should not report a hostname (the task/pod is the unit of identity, not the host).
func IsSidecar() bool {
	if env.IsFeaturePresent(env.ECSFargate) || env.IsFeaturePresent(env.EKSFargate) {
		return true
	}

	// Check if we're in ECS sidecar mode (includes managed instances in sidecar)
	return env.IsECSSidecarMode(pkgconfigsetup.Datadog())
}

// GetOrchestrator returns whether the Agent is running on ECS or EKS.
func GetOrchestrator() OrchestratorName {
	if env.IsFeaturePresent(env.EKSFargate) {
		return EKS
	}
	if env.IsFeaturePresent(env.ECSFargate) {
		return ECS
	}
	if env.IsFeaturePresent(env.ECSManagedInstances) {
		return ECSManagedInstances
	}
	return Unknown
}

// GetEKSFargateNodename returns the node name in EKS Fargate
func GetEKSFargateNodename() (string, error) {
	if nodename := pkgconfigsetup.Datadog().GetString("kubernetes_kubelet_nodename"); nodename != "" {
		return nodename, nil
	}
	return "", errors.New("kubernetes_kubelet_nodename is not defined, make sure DD_KUBERNETES_KUBELET_NODENAME is set via the downward API")
}
