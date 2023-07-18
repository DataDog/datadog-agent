// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build docker

package v3or4

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/util/ecs/common"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/telemetry"
)

const (
	// DefaultMetadataURIv3EnvVariable is the default environment variable used to hold the metadata endpoint URI v3.
	DefaultMetadataURIv3EnvVariable = "ECS_CONTAINER_METADATA_URI"
	// DefaultMetadataURIv4EnvVariable is the default environment variable used to hold the metadata endpoint URI v4.
	DefaultMetadataURIv4EnvVariable = "ECS_CONTAINER_METADATA_URI_V4"

	// Metadata v3 and v4 API paths. They're the same.
	taskMetadataPath         = "/task"
	taskMetadataWithTagsPath = "/taskWithTags"
)

// Client represents a client for a metadata v3 or v4 API endpoint.
type Client struct {
	agentURL   string
	apiVersion string
}

// NewClient creates a new client for the specified metadata v3 or v4 API endpoint.
func NewClient(agentURL, apiVersion string) *Client {
	return &Client{
		agentURL:   agentURL,
		apiVersion: apiVersion,
	}
}

// GetContainer returns metadata for a container.
func (c *Client) GetContainer(ctx context.Context) (*Container, error) {
	var ct Container
	if err := c.get(ctx, "", &ct); err != nil {
		return nil, err
	}
	return &ct, nil
}

// GetTask returns the current task.
func (c *Client) GetTask(ctx context.Context) (*Task, error) {
	return c.getTaskMetadataAtPath(ctx, taskMetadataPath)
}

// GetTaskWithTags returns the current task, including propagated resource tags.
func (c *Client) GetTaskWithTags(ctx context.Context) (*Task, error) {
	return c.getTaskMetadataAtPath(ctx, taskMetadataWithTagsPath)
}

func (c *Client) get(ctx context.Context, path string, v interface{}) error {
	client := http.Client{Timeout: common.MetadataTimeout()}
	url, err := c.makeURL(path)
	if err != nil {
		return fmt.Errorf("Error constructing metadata request URL: %s", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("Failed to create new request: %w", err)
	}
	resp, err := client.Do(req)

	defer func() {
		telemetry.AddQueryToTelemetry(path, resp)
	}()

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected HTTP status code in metadata %s reply: %d", c.apiVersion, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("Failed to decode metadata %s JSON payload to type %s: %s", c.apiVersion, reflect.TypeOf(v), err)
	}

	return nil
}

func (c *Client) getTaskMetadataAtPath(ctx context.Context, path string) (*Task, error) {
	var t Task
	if err := c.get(ctx, path, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Client) makeURL(requestPath string) (string, error) {
	u, err := url.Parse(c.agentURL)
	if err != nil {
		return "", err
	}
	// Unlike v1 and v2 the agent URL will contain a subpath that looks like
	// "/v3/<id>" so we must make sure not to dismiss the current URL path.
	u.Path = path.Join(u.Path, requestPath)
	return u.String(), nil
}
