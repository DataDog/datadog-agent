// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build docker

package ecs

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"time"
)

// GetTags returns host tags or static tags that are automatically added to
// ECS metrics
func GetTags(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	cluster, _, err := getECSMetadata(ctx)
	if err != nil {
		return nil, err
	}

	var hostTags []string

	if !pkgconfigsetup.Datadog().GetBool("disable_cluster_name_tag_key") {
		hostTags = append(hostTags, fmt.Sprintf("%s:%s", tags.ClusterName, cluster))
	}

	// always tag with ecs_cluster_name
	hostTags = append(hostTags, fmt.Sprintf("%s:%s", tags.EcsClusterName, cluster))

	return hostTags, nil
}
