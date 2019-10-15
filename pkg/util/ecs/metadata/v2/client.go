// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

// +build docker

package v2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
)

const (
	// ECS agent defaults
	defaultAgentURL = "http://169.254.170.2/"

	// Metadata v2 API paths
	taskMetadataPath         = "/metadata"
	taskMetadataWithTagsPath = "/metadataWithTags"
	containerStatsPath       = "/stats/"

	// Default client configuration
	endpointTimeout = 500 * time.Millisecond
)

// Client represents a client for a metadata v2 API endpoint.
type Client struct {
	agentURL string
}

// NewClient creates a new client for the specified metadata v2 API endpoint.
func NewClient(agentURL string) *Client {
	if !strings.HasSuffix(agentURL, "/") {
		agentURL += "/"
	}
	return &Client{
		agentURL: agentURL,
	}
}

// NewDefaultClient creates a new client for the default metadata v2 API endpoint.
func NewDefaultClient() *Client {
	return NewClient(defaultAgentURL)
}

// GetContainerStats returns stastics for a container.
func (c *Client) GetContainerStats(id string) (*ContainerStats, error) {
	var s ContainerStats
	if err := c.get(containerStatsPath+id, &s); err != nil {
		return nil, err
	}
	return &s, nil
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
		return fmt.Errorf("Unexpected HTTP status code in metadata v2 reply: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("Failed to decode metadata v2 JSON payload to type %s: %s", reflect.TypeOf(v), err)
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
	return fmt.Sprintf("%sv2%s", c.agentURL, path)
}
