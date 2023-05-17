// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build !docker

package v2

import "context"

// Client represents a client for a metadata v2 API endpoint.
type Client struct{}

// NewDefaultClient creates a new client for the default metadata v2 API endpoint.
func NewDefaultClient() *Client {
	return new(Client)
}

// GetTask returns the current task.
func (c *Client) GetTask(ctx context.Context) (*Task, error) {
	return new(Task), nil
}

// GetTaskWithTags returns the current task, including propagated resource tags.
func (c *Client) GetTaskWithTags(ctx context.Context) (*Task, error) {
	return new(Task), nil
}
