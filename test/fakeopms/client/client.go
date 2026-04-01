// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package client provides a Go client for the fake OPMS server control API.
// Tests use this to enqueue tasks and poll for results.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TaskResult is the result of a task published by PAR.
type TaskResult struct {
	TaskID       string                 `json:"task_id"`
	Success      bool                   `json:"success"`
	Outputs      map[string]interface{} `json:"outputs,omitempty"`
	ErrorCode    int                    `json:"error_code,omitempty"`
	ErrorDetails string                 `json:"error_details,omitempty"`
}

// Client talks to the fake OPMS control API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new fake OPMS client pointing at baseURL (e.g. "http://localhost:8080").
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Enqueue adds a task to the fake OPMS queue. PAR will pick it up on the next dequeue poll.
func (c *Client) Enqueue(taskID, actionFQN string, inputs map[string]interface{}) error {
	body := map[string]interface{}{
		"task_id":    taskID,
		"action_fqn": actionFQN,
		"inputs":     inputs,
	}
	return c.post("/fakeopms/enqueue", body)
}

// PollResult polls until PAR publishes a result for taskID or timeout elapses.
func (c *Client) PollResult(taskID string, timeout time.Duration) (*TaskResult, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		result, err := c.getResult(taskID)
		if err == nil {
			return result, nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("timed out waiting for result of task %s", taskID)
}

// Flush clears all pending tasks and results on the server.
func (c *Client) Flush() error {
	resp, err := c.httpClient.Post(c.baseURL+"/fakeopms/flush", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("flush returned %d", resp.StatusCode)
	}
	return nil
}

// GetHealth checks that the fake OPMS server is reachable.
func (c *Client) GetHealth() error {
	resp, err := c.httpClient.Get(c.baseURL + "/fakeopms/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) getResult(taskID string) (*TaskResult, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/fakeopms/result?taskID=%s", c.baseURL, taskID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no result yet")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result TaskResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) post(path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Post(c.baseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST %s returned %d", path, resp.StatusCode)
	}
	return nil
}
