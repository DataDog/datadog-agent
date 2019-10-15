// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

// +build docker

package v3

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
)

const (
	// Default environment variable used to hold the metadata endpoint URI.
	DefaultMetadataURIEnvVariable = "ECS_CONTAINER_METADATA_URI"

	// Metadata v3 API paths
	taskMetadataPath         = "/task"
	taskMetadataWithTagsPath = "/taskWithTags"
	containerStatsPath       = "/stats/"

	// Default client configuration
	endpointTimeout = 500 * time.Millisecond
)

// Client represents a client for a metadata v3 API endpoint.
type Client struct {
	agentURL string
}

// NewClient creates a new client for the specified metadata v3 API endpoint.
func NewClient(agentURL string) *Client {
	if strings.HasSuffix(agentURL, "/") {
		agentURL = strings.TrimSuffix(agentURL, "/")
	}
	return &Client{
		agentURL: agentURL,
	}
}

// GetContainer returns metadata for a container.
func (c *Client) GetContainer() (*Container, error) {
	var ct Container
	if err := c.get("", &ct); err != nil {
		return nil, err
	}
	return &ct, nil
}

// GetTask returns the current task.
func (c *Client) GetTask() (*Task, error) {
	return c.getTaskMetadataAtPath(taskMetadataPath)
}

// GetTaskWithTags returns the current task, including propagated resource tags.
func (c *Client) GetTaskWithTags() (*Task, error) {
	return c.getTaskMetadataAtPath(taskMetadataWithTagsPath)
}

func (c *Client) get(path string, v interface{}) error {
	client := http.Client{Timeout: endpointTimeout}
	url := c.makeURL(path)

	resp, err := client.Get(url)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected HTTP status code in metadata v3 reply: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("Failed to decode metadata v3 JSON payload to type %s: %s", reflect.TypeOf(v), err)
	}

	return nil
}

func (c *Client) getTaskMetadataAtPath(path string) (*Task, error) {
	var t Task
	if err := c.get(path, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Client) makeURL(path string) string {
	return fmt.Sprintf("%s%s", c.agentURL, path)
}
