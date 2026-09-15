// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogtelextensionimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

// ecsMetadataURIv4EnvVar is the environment variable ECS Fargate injects into every
// task, pointing at the Task Metadata Endpoint v4. pkg/util/ecs/metadata defines the
// same constant, but that package (like the rest of the workloadmeta ECS collector
// stack) is compiled only under the `docker` build tag, which otel-agent never sets
// (see tasks/build_tags.bzl's OTEL_AGENT_TAGS) -- so it is redefined here rather than
// imported.
const ecsMetadataURIv4EnvVar = "ECS_CONTAINER_METADATA_URI_V4"

const ecsMetadataRequestTimeout = 5 * time.Second

// ecsTaskMetadata is the subset of the ECS Task Metadata Endpoint v4 `/task` response
// this package needs.
type ecsTaskMetadata struct {
	TaskARN string `json:"TaskARN"`
}

// fetchECSTaskARN retrieves the current task's ARN directly from the ECS Task
// Metadata Endpoint v4, which Fargate automatically injects via
// ecsMetadataURIv4EnvVar -- no Docker/container-runtime access required.
func fetchECSTaskARN(ctx context.Context) (string, error) {
	baseURI := os.Getenv(ecsMetadataURIv4EnvVar)
	if baseURI == "" {
		return "", fmt.Errorf("%s is not set", ecsMetadataURIv4EnvVar)
	}

	ctx, cancel := context.WithTimeout(ctx, ecsMetadataRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURI+"/task", nil)
	if err != nil {
		return "", fmt.Errorf("failed to build ECS task metadata request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch ECS task metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ECS task metadata endpoint returned status %d", resp.StatusCode)
	}

	var task ecsTaskMetadata
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return "", fmt.Errorf("failed to decode ECS task metadata: %w", err)
	}
	if task.TaskARN == "" {
		return "", errors.New("ECS task metadata response did not contain a TaskARN")
	}

	return task.TaskARN, nil
}
