// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ec2 && docker

package tags

import (
	"context"
	"fmt"

	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
)

// getContainerInstanceARN fetches the ARN of the ECS container instance using the
// ECS metadata introspection endpoint (metadata v1).
func getContainerInstanceARN(ctx context.Context) (string, error) {
	client, err := ecsmeta.V1()
	if err != nil {
		return "", fmt.Errorf("unable to initialize ECS metadata client: %w", err)
	}

	instance, err := client.GetInstance(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to query ECS metadata: %w", err)
	}

	return instance.ContainerInstanceARN, nil
}
