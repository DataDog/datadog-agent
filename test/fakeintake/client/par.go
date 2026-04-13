// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/server"
)

// EnqueuePARTask enqueues a task for the Private Action Runner to dequeue and execute.
// taskID must be a unique identifier (e.g. uuid.New().String()).
// actionFQN is the fully-qualified action name (e.g. "com.datadoghq.remoteaction.rshell.runCommand").
func (c *Client) EnqueuePARTask(taskID, actionFQN string, inputs map[string]interface{}) error {
	body, err := json.Marshal(map[string]interface{}{
		"task_id":    taskID,
		"action_fqn": actionFQN,
		"inputs":     inputs,
	})
	if err != nil {
		return fmt.Errorf("marshal enqueue request: %w", err)
	}
	resp, err := http.Post(c.fakeIntakeURL+"/fakeintake/par/enqueue", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("enqueue PAR task: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("enqueue PAR task: status %d: %s", resp.StatusCode, b)
	}
	return nil
}

// GetPARTaskResult polls fakeintake for the result of the given task until it appears
// or the timeout expires.
func (c *Client) GetPARTaskResult(taskID string, timeout time.Duration) (*server.PARTaskResult, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		result, err := c.getPARResult(taskID)
		if err == nil {
			return result, nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("timed out waiting for result of task %s", taskID)
}

// FlushPAR clears all queued PAR tasks and captured results from fakeintake.
func (c *Client) FlushPAR() error {
	resp, err := http.Post(c.fakeIntakeURL+"/fakeintake/par/flush", "", nil)
	if err != nil {
		return fmt.Errorf("flush PAR state: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("flush PAR state: status %d", resp.StatusCode)
	}
	return nil
}

// GetPARDequeueCount returns how many times PAR has called the dequeue endpoint.
// A non-zero value confirms PAR is actively polling fakeintake.
func (c *Client) GetPARDequeueCount() (int, error) {
	resp, err := http.Get(c.fakeIntakeURL + "/fakeintake/par/stats")
	if err != nil {
		return 0, fmt.Errorf("get PAR stats: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("get PAR stats: status %d", resp.StatusCode)
	}
	var stats struct {
		DequeueCalls int `json:"dequeue_calls"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return 0, fmt.Errorf("decode PAR stats: %w", err)
	}
	return stats.DequeueCalls, nil
}

func (c *Client) getPARResult(taskID string) (*server.PARTaskResult, error) {
	resp, err := http.Get(fmt.Sprintf("%s/fakeintake/par/result?taskID=%s", c.fakeIntakeURL, taskID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("no result yet")
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get PAR result: status %d: %s", resp.StatusCode, b)
	}
	var result server.PARTaskResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode PAR result: %w", err)
	}
	return &result, nil
}
