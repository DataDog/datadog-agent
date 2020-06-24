// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build fargateprocess

package fargate

import (
	"errors"
	"fmt"

	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
)

// GetFargateHost returns the hostname to be used
// by the process Agent based on the Fargate orchestrator
// - ECS: fargate_task:<TaskARN>
// - EKS: value of kubernetes_kubelet_nodename
func GetFargateHost() (string, error) {
	return getFargateHost(GetOrchestrator(), getECSHost, getEKSHost)
}

// getFargateHost is separated from GetFargateHost for testing purpose
func getFargateHost(orchestrator OrchestratorName, ecsFunc, eksFunc func() (string, error)) (string, error) {
	// Fargate should have no concept of host names
	// we set the hostname depending on the orchestrator
	switch orchestrator {
	case ECS:
		return ecsFunc()
	case EKS:
		return eksFunc()
	}
	return "", errors.New("unknown Fargate orchestrator")
}

func getECSHost() (string, error) {
	// Use the task ARN as hostname
	taskMeta, err := ecsmeta.V2().GetTask()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("fargate_task:%s", taskMeta.TaskARN), nil
}

func getEKSHost() (string, error) {
	// Use the node name as hostname
	return GetEKSFargateNodename()
}
