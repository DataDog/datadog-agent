// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// +build docker

package v2

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/util/ecs/common"
)

const (
	// ECS agent defaults
	defaultAgentURL = "http://169.254.170.2"

	// Metadata v2 API paths
	taskMetadataPath         = "/metadata"
	taskMetadataWithTagsPath = "/metadataWithTags"
	containerStatsPath       = "/stats"
)

// Client represents a client for a metadata v2 API endpoint.
type Client struct {
	agentURL string
}

// NewClient creates a new client for the specified metadata v2 API endpoint.
func NewClient(agentURL string) *Client {
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
	var stats map[string]*ContainerStats
	// There is a difference in reported JSON in v 1.4.0 vs v1.3.0 so we should
	// avoid using /v2/stats/{container_id}
	if err := c.get(containerStatsPath, &stats); err != nil {
		return nil, err
	}

	if s, ok := stats[id]; ok && s != nil {
		return s, nil
	}

	return nil, fmt.Errorf("Failed to retrieve container stats for id: %s", id)
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
		var msg string
		if buf, err := ioutil.ReadAll(resp.Body); err == nil {
			msg = string(buf)
		}
		return fmt.Errorf("Unexpected HTTP status code in metadata v2 reply: [%s]: %d - %s", url, resp.StatusCode, msg)
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

func (c *Client) makeURL(requestPath string) (string, error) {
	u, err := url.Parse(c.agentURL)
	if err != nil {
		return "", err
	}
	u.Path = path.Join("/v2", requestPath)
	return u.String(), nil
}
