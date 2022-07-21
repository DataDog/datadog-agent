// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator
// +build orchestrator

package k8s

import model "github.com/DataDog/agent-payload/v5/process"

// build orchestrator manifest message
func buildManifestMessageBody(kubeClusterName, clusterID string, msgGroupID int32, resourceManifests []interface{}, groupSize int) model.MessageBody {
	manifests := make([]*model.Manifest, 0, len(resourceManifests))

	for _, m := range resourceManifests {
		manifests = append(manifests, m.(*model.Manifest))
	}

	return &model.CollectorManifest{
		ClusterName: kubeClusterName,
		ClusterId:   clusterID,
		Manifests:   manifests,
		GroupId:     msgGroupID,
		GroupSize:   int32(groupSize),
	}
}
