// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"encoding/json"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

// ECSMetadata defines ECS metadatas
type ECSMetadata struct {
	DockerID   string `json:"DockerId"`
	DockerName string `json:"DockerName"`
	Name       string `json:"Name"`
}

func retrieveECSMetadata(url string) (*ECSMetadata, error) {
	body, err := simpleHTTPRequest(url)
	if err != nil {
		return nil, err
	}

	data := ECSMetadata{}
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	return &data, nil
}

func retrieveEnvMetadata(ctx *ebpfless.ContainerContext) {
	if id := os.Getenv("DD_CONTAINER_ID"); id != "" {
		ctx.ID = id
	}

	if name := os.Getenv("DD_CONTAINER_NAME"); name != "" {
		ctx.Name = name
	}
}

func newContainerContext(containerID string) (*ebpfless.ContainerContext, error) {
	ctx := &ebpfless.ContainerContext{
		ID: containerID,
	}

	if ecsContainerMetadataURI := os.Getenv("ECS_CONTAINER_METADATA_URI_V4"); ecsContainerMetadataURI != "" {
		data, err := retrieveECSMetadata(ecsContainerMetadataURI)
		if err != nil {
			return nil, err
		}
		if data != nil {
			if data.DockerID != "" && ctx.ID == "" {
				// only set the container ID if we previously failed to retrieve it from proc
				ctx.ID = data.DockerID
			}
			if data.DockerName != "" {
				ctx.Name = data.DockerName
			}
		}
	}
	retrieveEnvMetadata(ctx)

	ctx.CreatedAt = uint64(time.Now().UnixNano())

	return ctx, nil
}
