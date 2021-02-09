// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build docker

package ecs

import (
	"fmt"

	payload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/metadata"

	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
)

// GetPayload returns a payload.ECSMetadataPayload with metadata about the state
// of the local ECS containers running on this node. This data is provided via
// the local ECS agent.
func GetPayload() (metadata.Payload, error) {
	if ecsutil.IsFargateInstance() {
		return nil, fmt.Errorf("ECS metadata disabled on Fargate")
	}

	metaV1, err := ecsmeta.V1()
	if err != nil {
		return nil, err
	}
	tasks, err := metaV1.GetTasks()
	if err != nil {
		return nil, err
	}
	return buildPayload(tasks), nil
}

func buildPayload(tasks []v1.Task) *payload.ECSMetadataPayload {
	pt := make([]*payload.ECSMetadataPayload_Task, 0, len(tasks))
	for _, t := range tasks {
		containers := make([]*payload.ECSMetadataPayload_Container, 0, len(t.Containers))
		for _, c := range t.Containers {
			containers = append(containers, &payload.ECSMetadataPayload_Container{
				DockerId:   c.DockerID,
				DockerName: c.DockerName,
				Name:       c.Name,
			})
		}
		pt = append(pt, &payload.ECSMetadataPayload_Task{
			Arn:           t.Arn,
			DesiredStatus: t.DesiredStatus,
			KnownStatus:   t.KnownStatus,
			Family:        t.Family,
			Version:       t.Version,
			Containers:    containers,
		})
	}
	return &payload.ECSMetadataPayload{Tasks: pt}
}
