// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fargate

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// IsFargateInstance returns whether the Agent is running in Fargate.
func IsFargateInstance() bool {
	return config.IsFeaturePresent(config.ECSFargate) || config.IsFeaturePresent(config.EKSFargate)
}

// GetOrchestrator returns whether the Agent is running on ECS or EKS.
func GetOrchestrator() OrchestratorName {
	if config.IsFeaturePresent(config.EKSFargate) {
		return EKS
	}
	if config.IsFeaturePresent(config.ECSFargate) {
		return ECS
	}
	return Unknown
}

// GetEKSFargateNodename returns the node name in EKS Fargate
func GetEKSFargateNodename() (string, error) {
	if nodename := config.Datadog().GetString("kubernetes_kubelet_nodename"); nodename != "" {
		return nodename, nil
	}
	return "", errors.New("kubernetes_kubelet_nodename is not defined, make sure DD_KUBERNETES_KUBELET_NODENAME is set via the downward API")
}
