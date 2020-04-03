// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package fargate

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
)

// IsFargateInstance returns whether the Agent is running in Fargate.
func IsFargateInstance() bool {
	return ecs.IsFargateInstance() || IsEKSFargateInstance()
}

// GetOrchestrator returns whether the Agent is running on ECS or EKS.
func GetOrchestrator() OrchestratorName {
	if IsEKSFargateInstance() {
		return EKS
	}
	if ecs.IsFargateInstance() {
		return ECS
	}
	return Unknown
}

// IsEKSFargateInstance returns whether the Agent is running in EKS Fargate.
func IsEKSFargateInstance() bool {
	return config.Datadog.GetBool("eks_fargate")
}

// GetEKSFargateNodename returns the node name in EKS Fargate
func GetEKSFargateNodename() (string, error) {
	if nodename := config.Datadog.GetString("kubernetes_kubelet_nodename"); nodename != "" {
		return nodename, nil
	}
	return "", errors.New("kubernetes_kubelet_nodename is not defined, make sure DD_KUBERNETES_KUBELET_NODENAME is set via the downward API")
}
