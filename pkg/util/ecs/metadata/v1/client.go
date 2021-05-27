// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// +build docker

package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/util/ecs/common"
)

const (
	// DefaultAgentPort is the default introspection endpoint port.
	DefaultAgentPort = 51678

	// Metadata v1 API paths
	instancePath     = "/metadata"
	taskMetadataPath = "/tasks"
)

// Client represents a client for a metadata v1 API endpoint.
type Client struct {
	agentURL string
}

// NewClient creates a new client for the specified metadata v1 API endpoint.
func NewClient(agentURL string) *Client {
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

func (c *Client) makeURL(requestPath string) (string, error) {
	u, err := url.Parse(c.agentURL)
	if err != nil {
		return "", err
	}
	u.Path = path.Join("/v1", requestPath)
	return u.String(), nil
}

func (c *Client) get(path string, v interface{}) error {
	client := http.Client{Timeout: common.MetadataTimeout()}
	url, err := c.makeURL(path)
	if err != nil {
		return fmt.Errorf("Error constructing metadata request URL: %s", err)
	}

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
