// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
)

// ECSMetadata defines ECS metadata
// https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-metadata-endpoint-v4.html
type ECSMetadata struct {
	DockerID string `json:"DockerId"`
}

func retrieveECSMetadata(url string) (*ECSMetadata, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get ECS metadata endpoint response: %w", err)
	}

	body, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read ECS metadata endpoint response: %w", err)
	}

	if res.StatusCode > 299 {
		return nil, fmt.Errorf("ECS metadata endpoint returned an invalid http code: %d", res.StatusCode)
	}

	data := ECSMetadata{}
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	return &data, nil
}

func retrieveEnvMetadata(ctx *ebpfless.ContainerContext) {
	if id := os.Getenv("DD_CONTAINER_ID"); id != "" {
		ctx.ID = containerutils.ContainerID(id)
	}
}

func newContainerContext(containerID containerutils.ContainerID) (*ebpfless.ContainerContext, error) {
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
				ctx.ID = containerutils.ContainerID(data.DockerID)
			}
		}
	}
	retrieveEnvMetadata(ctx)

	ctx.CreatedAt = uint64(time.Now().UnixNano())

	return ctx, nil
}
