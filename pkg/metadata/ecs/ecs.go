// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package ecs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	payload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	dockerutil "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/docker/docker/client"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
)

// GetPayload returns a payload.ECSMetadataPayload with metadat about the state
// of the local ECS containers running on this node. This data is provided via
// the local ECS agent.
func GetPayload() (metadata.Payload, error) {
	resp, err := ecsutil.ExtractPayload()
	parseTaskResponse(resp)
}


func parseTaskResponse(resp ecsutil.TasksV1Response) *payload.ECSMetadataPayload {
	tasks := make([]*payload.ECSMetadataPayload_Task, 0, len(resp.Tasks))
	for _, t := range resp.Tasks {
		containers := make([]*payload.ECSMetadataPayload_Container, 0, len(t.Containers))
		for _, c := range t.Containers {
			containers = append(containers, &payload.ECSMetadataPayload_Container{
				DockerId:   c.DockerID,
				DockerName: c.DockerName,
				Name:       c.Name,
			})
		}

		tasks = append(tasks, &payload.ECSMetadataPayload_Task{
			Arn:           t.Arn,
			DesiredStatus: t.DesiredStatus,
			KnownStatus:   t.KnownStatus,
			Family:        t.Family,
			Version:       t.Version,
			Containers:    containers,
		})
	}
	return &payload.ECSMetadataPayload{Tasks: tasks}
}
