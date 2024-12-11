// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build docker

// Package v3or4 provides an ECS client for the version v3 and v4 of the API.
package v3or4

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/DataDog/datadog-agent/pkg/util/ecs/common"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// Client is an interface for ECS metadata v3 and v4 API clients.
type Client interface {
	GetTask(ctx context.Context) (*Task, error)
	GetContainer(ctx context.Context) (*Container, error)
	GetTaskWithTags(ctx context.Context) (*Task, error)
}

// Client represents a client for a metadata v3 or v4 API endpoint.
type client struct {
	agentURL   string
	apiVersion string

	retry                  bool
	initialInterval        time.Duration                     // initialInterval is the initial interval between retries.
	maxElapsedTime         time.Duration                     // maxElapsedTime is the maximum time to retry before giving up.
	increaseRequestTimeout func(time.Duration) time.Duration // increaseRequestTimeout is a function that increases the request timeout on each retry.

}

// ClientOption represents an option to configure the client.
type ClientOption func(*client)

// WithTryOption configures the client to retry requests with an exponential backoff.
func WithTryOption(initialInterval, maxElapsedTime time.Duration, increaseRequestTimeout func(time.Duration) time.Duration) ClientOption {
	return func(c *client) {
		c.retry = true
		c.initialInterval = initialInterval
		c.maxElapsedTime = maxElapsedTime
		c.increaseRequestTimeout = increaseRequestTimeout
	}
}

// NewClient creates a new client for the specified metadata v3 or v4 API endpoint.
func NewClient(agentURL, apiVersion string, options ...ClientOption) Client {
	c := &client{
		agentURL:   agentURL,
		apiVersion: apiVersion,
	}

	for _, op := range options {
		op(c)
	}

	return c
}

// GetContainer returns metadata for a container.
func (c *client) GetContainer(ctx context.Context) (*Container, error) {
	var ct Container
	if err := c.get(ctx, "", &ct); err != nil {
		return nil, err
	}
	return &ct, nil
}

// GetTask returns the current task.
func (c *client) GetTask(ctx context.Context) (*Task, error) {
	return c.getTaskMetadataAtPath(ctx, taskMetadataPath)
}

// GetTaskWithTags returns the current task, including propagated resource tags.
func (c *client) GetTaskWithTags(ctx context.Context) (*Task, error) {
	return c.getTaskMetadataAtPath(ctx, taskMetadataWithTagsPath)
}

func (c *client) get(ctx context.Context, path string, v interface{}) error {
	client := http.Client{Timeout: common.MetadataTimeout()}
	url, err := c.makeURL(path)
	if err != nil {
		return fmt.Errorf("Error constructing metadata request URL: %s", err)
	}

	var resp *http.Response
	operation := func() error {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("Failed to create new request: %w", err)
		}
		resp, err = client.Do(req)

		defer func() {
			telemetry.AddQueryToTelemetry(path, resp)
		}()

		if err != nil {
			if os.IsTimeout(err) && c.retry {
				client.Timeout = c.increaseRequestTimeout(client.Timeout)
				log.Debugf("Timeout while querying metadata %s, increasing timeout to %s", url, client.Timeout)
			}
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

	// retry is false by default
	if !c.retry {
		return operation()
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = c.initialInterval
	expBackoff.MaxElapsedTime = c.maxElapsedTime

	return backoff.Retry(operation, expBackoff)
}

func (c *client) getTaskMetadataAtPath(ctx context.Context, path string) (*Task, error) {
	var t Task
	if err := c.get(ctx, path, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *client) makeURL(requestPath string) (string, error) {
	u, err := url.Parse(c.agentURL)
	if err != nil {
		return "", err
	}
	// Unlike v1 and v2 the agent URL will contain a subpath that looks like
	// "/v3/<id>" so we must make sure not to dismiss the current URL path.
	u.Path = path.Join(u.Path, requestPath)
	return u.String(), nil
}
