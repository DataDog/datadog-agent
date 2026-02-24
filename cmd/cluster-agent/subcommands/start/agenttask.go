// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package start

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/remote-config/functiontools"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	agentTaskTimeout = 5 * time.Minute
)

// agentTaskHandler is responsible for processing AGENT_TASK configurations
// received via remote config
type agentTaskHandler struct {
	m             *sync.Mutex
	taskProcessed map[string]bool
}

// newAgentTaskHandler creates a new agent task handler
func newAgentTaskHandler() *agentTaskHandler {
	return &agentTaskHandler{
		m:             &sync.Mutex{},
		taskProcessed: make(map[string]bool),
	}
}

// handleAgentTaskUpdate is the callback function called when there is an AGENT_TASK config update
func (h *agentTaskHandler) handleAgentTaskUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	wg := &sync.WaitGroup{}
	wg.Add(len(updates))

	pkglog.Infof("Received %d AGENT_TASK configuration(s) from remote config", len(updates))

	// Execute all AGENT_TASK in separate goroutines, so we don't block if one of them deadlocks
	for originalConfigPath, originalConfig := range updates {
		go func(configPath string, c state.RawConfig) {
			pkglog.Infof("Agent task %s started", configPath)
			defer wg.Done()
			defer pkglog.Infof("Agent task %s completed", configPath)

			task, parseErr := types.ParseConfigAgentTask(c.Config, c.Metadata)
			if parseErr != nil {
				pkglog.Errorf("Failed to parse agent task %s: %v", configPath, parseErr)
				applyStateCallback(configPath, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: parseErr.Error(),
				})
				return
			}

			h.m.Lock()
			// Check that the task wasn't already processed
			if h.taskProcessed[task.Config.UUID] {
				pkglog.Infof("Agent task %s (UUID: %s) already processed, skipping", configPath, task.Config.UUID)
				h.m.Unlock()
				return
			}
			h.taskProcessed[task.Config.UUID] = true
			h.m.Unlock()

			// Mark it as unacknowledged first
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateUnacknowledged,
			})

			var execErr error
			processed := false

			// Handle different task types
			switch types.TaskType(task.Config.TaskType) {
			case types.TaskExecuteTool:
				processed = true
				pkglog.Infof("Executing function tool for agent task %s", configPath)
				pkglog.Debugf("Task details - Call ID: %s, Function: %s",
					task.Config.TaskArgs["call_id"],
					task.Config.TaskArgs["function_tool_name"])

				execErr = functiontools.NewCall(task).Execute().Send()
				if execErr != nil {
					pkglog.Errorf("Error while executing function tool for agent task %s: %v", configPath, execErr)
				} else {
					pkglog.Infof("Successfully executed function tool for agent task %s", configPath)
				}

			default:
				pkglog.Warnf("Unsupported task type '%s' for agent task %s", task.Config.TaskType, configPath)
				applyStateCallback(configPath, state.ApplyStatus{
					State: state.ApplyStateUnknown,
				})
				return
			}

			// Report status based on execution result
			if processed && execErr != nil {
				applyStateCallback(configPath, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: execErr.Error(),
				})
			} else if processed && execErr == nil {
				applyStateCallback(configPath, state.ApplyStatus{
					State: state.ApplyStateAcknowledged,
				})
			} else {
				applyStateCallback(configPath, state.ApplyStatus{
					State: state.ApplyStateUnknown,
				})
			}
		}(originalConfigPath, originalConfig)
	}

	// Check if one of the tasks reaches timeout
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()

	select {
	case <-c:
		// completed normally
		pkglog.Infof("All %d agent task(s) completed successfully", len(updates))
	case <-time.After(agentTaskTimeout):
		// timed out
		pkglog.Errorf("Timeout: at least one agent task did not complete within %v", agentTaskTimeout)
	}
}
