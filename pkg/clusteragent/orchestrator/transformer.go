// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver,orchestrator

package orchestrator

import (
	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/util/orchestrator"

	v1 "k8s.io/api/apps/v1"
)

func extractDeployment(d *v1.Deployment) *model.Deployment {
	deploy := model.Deployment{
		Metadata: orchestrator.ExtractMetadata(&d.ObjectMeta),
	}

	return &deploy
}

func extractReplicaset(rs *v1.ReplicaSet) *model.ReplicaSet {
	replicaSet := model.ReplicaSet{
		Metadata: orchestrator.ExtractMetadata(&rs.ObjectMeta),
	}

	return &replicaSet
}
