// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clusterinfo provides utilities to retrieve cluster information from the Cluster Agent
package clusterinfo

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

// GetClusterAgentStaticTags gets the tags from the cluster-agent
// Currently only ClusterID for orch_cluster_id tag
// Context is passed to match the host tags interface
func GetClusterAgentStaticTags(context.Context) ([]string, error) {
	clusterID, err := clustername.GetClusterID()
	if err != nil {
		return nil, err
	}

	if clusterID != "" {
		return []string{tags.OrchClusterID + ":" + clusterID}, nil
	}

	return nil, nil
}
