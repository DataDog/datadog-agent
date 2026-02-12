// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package autoconnections

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type kubernetesTagsProvider struct {
	cfg model.Reader
}

// NewTagsProvider creates a TagsProvider for building connection tags
func NewTagsProvider(cfg model.Reader) TagsProvider {
	return &kubernetesTagsProvider{cfg: cfg}
}

func (p *kubernetesTagsProvider) GetTags(ctx context.Context, runnerID, hostname string) []string {
	tags := []string{
		"runner-id:" + runnerID,
		"hostname:" + hostname,
	}

	// Only attempt to get cluster tags if cluster_agent is enabled
	if p.cfg.GetBool("cluster_agent.enabled") {
		if clusterID, err := clustername.GetClusterID(); err == nil && clusterID != "" {
			tags = append(tags, "cluster-id:"+clusterID)
		} else if err != nil {
			log.Debugf("Failed to get cluster ID: %v", err)
		}

		if clusterName := clustername.GetClusterName(ctx, hostname); clusterName != "" {
			tags = append(tags, "cluster-name:"+clusterName)
		}
	}

	return tags
}
