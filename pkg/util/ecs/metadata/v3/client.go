// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// +build docker

package v3

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
	// DefaultMetadataURIEnvVariable is the default environment variable used to hold the metadata endpoint URI.
	DefaultMetadataURIEnvVariable = "ECS_CONTAINER_METADATA_URI"

	// Metadata v3 API paths
	taskMetadataPath         = "/task"
	taskMetadataWithTagsPath = "/taskWithTags"
)

// Client represents a client for a metadata v3 API endpoint.
type Client struct {
	agentURL string
}

// NewClient creates a new client for the specified metadata v3 API endpoint.
func NewClient(agentURL string) *Client {
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
