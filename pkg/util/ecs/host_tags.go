// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build docker

package ecs

import (
	"context"
	"time"
)

// GetTags returns tags that are automatically added to metrics and events on a
// host that is running in ECS
func GetTags(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	ecsMeta, _, err := getECSInstanceMetadata(ctx)
	if err != nil {
		return nil, err
	}

	return []string{"ecs_cluster_name" + ecsMeta}, nil
}
