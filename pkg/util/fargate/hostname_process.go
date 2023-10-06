// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build fargateprocess

package fargate

import (
	"context"
	"errors"
	"fmt"

	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetFargateHost returns the hostname to be used
// by the process Agent based on the Fargate orchestrator
// - ECS: fargate_task:<TaskARN>
// - EKS: value of kubernetes_kubelet_nodename
func GetFargateHost(ctx context.Context) (string, error) {
	return getFargateHost(ctx, GetOrchestrator(), getECSHost, getEKSHost)
}

// getFargateHost is separated from GetFargateHost for testing purpose
func getFargateHost(ctx context.Context, orchestrator OrchestratorName, ecsFunc, eksFunc func(context.Context) (string, error)) (string, error) {
	// Fargate should have no concept of host names
	// we set the hostname depending on the orchestrator
	switch orchestrator {
	case ECS:
		return ecsFunc(ctx)
	case EKS:
		return eksFunc(ctx)
	}
	return "", errors.New("unknown Fargate orchestrator")
}

func getECSHost(ctx context.Context) (string, error) {
	client, err := ecsmeta.V2()
	if err != nil {
		log.Debugf("error while initializing ECS metadata V2 client: %s", err)
		return "", err
	}

	// Use the task ARN as hostname
	taskMeta, err := client.GetTask(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("fargate_task:%s", taskMeta.TaskARN), nil
}

func getEKSHost(ctx context.Context) (string, error) {
	// Use the node name as hostname
	return GetEKSFargateNodename()
}
