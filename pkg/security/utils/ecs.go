// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
)

// ECSMetadataURIv4EnvVar is the environment variable holding the task metadata endpoint v4 of the current container
const ECSMetadataURIv4EnvVar = "ECS_CONTAINER_METADATA_URI_V4"

// GetECSTaskID returns the ID of the ECS task of the container whose task metadata endpoint v4 is metadataURI
func GetECSTaskID(ctx context.Context, metadataURI string) (string, error) {
	taskURL, err := url.Parse(metadataURI)
	if err != nil {
		return "", err
	}
	taskURL.Path = path.Join(taskURL.Path, "task")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, taskURL.String(), nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code from the task metadata endpoint: %d", resp.StatusCode)
	}

	var task struct {
		TaskARN string `json:"TaskARN"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return "", fmt.Errorf("unable to decode the task metadata: %w", err)
	}

	taskID := containerutils.ECSTaskIDFromARN(task.TaskARN)
	if taskID == "" {
		return "", fmt.Errorf("invalid task ARN: %q", task.TaskARN)
	}

	return taskID, nil
}
