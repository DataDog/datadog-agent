// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

// +build docker

package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
)

const (
	// Default introspection endpoint port.
	DefaultAgentPort = 51678

	// Metadata v1 API paths
	commandsPath     = "/"
	instancePath     = "/metadata"
	taskMetadataPath = "/tasks"

	// Default client configuration
	endpointTimeout = 500 * time.Millisecond
)

// Client represents a client for a metadata v1 API endpoint.
type Client struct {
	agentURL string
}

// NewClient creates a new client for the specified metadata v1 API endpoint.
func NewClient(agentURL string) *Client {
	if !strings.HasSuffix(agentURL, "/") {
		agentURL += "/"
	}
	return &Client{
		agentURL: agentURL,
	}
}

// GetInstance returns metadata for the current container instance.
func (c *Client) GetInstance() (*Instance, error) {
	var i Instance
	if err := c.get(instancePath, &i); err != nil {
		return nil, err
	}
	return &i, nil
}

// GetTasks returns the list of task on the current container instance.
func (c *Client) GetTasks() ([]Task, error) {
	var t Tasks
	if err := c.get(taskMetadataPath, &t); err != nil {
		return nil, err
	}
	return t.Tasks, nil
}

func (c *Client) makeURL(path string) string {
	return fmt.Sprintf("%sv1%s", c.agentURL, path)
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
		return fmt.Errorf("Unexpected HTTP status code in metadata v1 reply: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return fmt.Errorf("Failed to decode metadata v1 JSON payload to type %s: %s", reflect.TypeOf(v), err)
	}

	return nil
}
