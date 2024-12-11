// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package executioncontext

import (
	"os"
	"strings"
	"sync"
	"time"

	json "github.com/json-iterator/go"
)

const persistedStateFilePath = "/tmp/dd-lambda-extension-cache.json"

//nolint:revive // TODO(SERV) Fix revive linter
type ColdStartTags struct {
	IsColdStart     bool
	IsProactiveInit bool
}

// ExecutionContext represents the execution context
type ExecutionContext struct {
	m                  sync.Mutex
	arn                string
	lastRequestID      string
	coldstartRequestID string
	wasColdStart       bool
	wasProactiveInit   bool
	lastLogRequestID   string
	lastOOMRequestID   string
	runtime            string
	initTime           time.Time
	startTime          time.Time
	endTime            time.Time

	persistedStateFilePath string
	isStateSaved           bool
}

// State represents the state of the execution context at a point in time
type State struct {
	ARN                string
	LastRequestID      string
	ColdstartRequestID string
	WasColdStart       bool
	WasProactiveInit   bool
	LastLogRequestID   string
	LastOOMRequestID   string
	Runtime            string
	InitTime           time.Time
	StartTime          time.Time
	EndTime            time.Time
}

// GetCurrentState gets the current state of the execution context
func (ec *ExecutionContext) GetCurrentState() State {
	ec.m.Lock()
	defer ec.m.Unlock()
	return State{
		ARN:                ec.arn,
		LastRequestID:      ec.lastRequestID,
		ColdstartRequestID: ec.coldstartRequestID,
		WasColdStart:       ec.wasColdStart,
		WasProactiveInit:   ec.wasProactiveInit,
		LastLogRequestID:   ec.lastLogRequestID,
		LastOOMRequestID:   ec.lastOOMRequestID,
		Runtime:            ec.runtime,
		InitTime:           ec.initTime,
		StartTime:          ec.startTime,
		EndTime:            ec.endTime,
	}
}

// Returns whether or not the given request ID is a cold start or is a proactive init
//
//nolint:revive // TODO(SERV) Fix revive linter
func (ec *ExecutionContext) GetColdStartTagsForRequestID(requestID string) ColdStartTags {
	ec.m.Lock()
	defer ec.m.Unlock()
	coldStartTags := ColdStartTags{
		IsColdStart:     false,
		IsProactiveInit: false,
	}
	if requestID != ec.coldstartRequestID {
		return coldStartTags
	}

	coldStartTags.IsColdStart = ec.wasColdStart
	coldStartTags.IsProactiveInit = ec.wasProactiveInit
	return coldStartTags
}

// LastRequestID return the last seen request identifier through the extension API.
func (ec *ExecutionContext) LastRequestID() string {
	ec.m.Lock()
	defer ec.m.Unlock()
	return ec.lastRequestID
}

// SetFromInvocation sets the execution context based on an invocation
func (ec *ExecutionContext) SetFromInvocation(arn string, requestID string) {
	ec.m.Lock()
	defer ec.m.Unlock()
	ec.arn = strings.ToLower(arn)
	ec.lastRequestID = requestID
	if len(ec.coldstartRequestID) == 0 {
		// Putting this within the guard clause of
		// coldstartRequestID == 0 ensures this is set once
		// 10 seconds is the maximimum init time, so if the difference
		// between the first invocation start time and initTime
		// is greater than 10, we're in a proactive_initialization
		// not a cold start

		// TODO Astuyve - refactor this to use initTime
		// from TelemetryAPI
		if time.Since(ec.initTime) > 10*time.Second {
			ec.wasProactiveInit = true
			ec.wasColdStart = false
		} else {
			ec.wasColdStart = true
		}
		ec.coldstartRequestID = requestID
	}
}

// SetArnFromExtensionResponse sets the execution context from the Extension API response
func (ec *ExecutionContext) SetArnFromExtensionResponse(arn string) {
	ec.m.Lock()
	defer ec.m.Unlock()
	ec.arn = strings.ToLower(arn)
}

// SetInitTime sets the agent initialization time
//
//nolint:revive // TODO(SERV) Fix revive linter
func (ec *ExecutionContext) SetInitializationTime(time time.Time) {
	ec.m.Lock()
	defer ec.m.Unlock()
	ec.initTime = time
}

// UpdateStartTime updates the execution context based on a platform.Start log message
func (ec *ExecutionContext) UpdateStartTime(time time.Time) {
	ec.m.Lock()
	defer ec.m.Unlock()
	ec.startTime = time
}

// UpdateEndTime updates the execution context based on a
// platform.runtimeDone log message
func (ec *ExecutionContext) UpdateEndTime(time time.Time) {
	ec.m.Lock()
	defer ec.m.Unlock()
	ec.endTime = time
}

// UpdateOutOfMemoryRequestID updates the execution context with the request ID if an
// out of memory is detected either in the function or platform.report logs
//
//nolint:revive // TODO(SERV) Fix revive linter
func (ec *ExecutionContext) UpdateOutOfMemoryRequestID(requestId string) {
	ec.m.Lock()
	defer ec.m.Unlock()
	ec.lastOOMRequestID = requestId
}

// UpdateRuntime updates the execution context with the runtime information
func (ec *ExecutionContext) UpdateRuntime(runtime string) {
	ec.m.Lock()
	defer ec.m.Unlock()
	ec.runtime = runtime
}

// getPersistedStateFilePath returns the full path and filename of the
// persisted state file.
func (ec *ExecutionContext) getPersistedStateFilePath() string {
	filepath := ec.persistedStateFilePath
	if filepath == "" {
		filepath = persistedStateFilePath
	}
	return filepath
}

// UpdatePersistedStateFilePath sets the path of the persisted state file
func (ec *ExecutionContext) UpdatePersistedStateFilePath(path string) {
	ec.m.Lock()
	defer ec.m.Unlock()
	ec.persistedStateFilePath = path
}

// SaveCurrentExecutionContext stores the current context to a file
func (ec *ExecutionContext) SaveCurrentExecutionContext() error {
	ecs := ec.GetCurrentState()
	file, err := json.Marshal(ecs)
	if err != nil {
		return err
	}
	filepath := ec.getPersistedStateFilePath()
	err = os.WriteFile(filepath, file, 0600)
	if err != nil {
		return err
	}
	ec.isStateSaved = true
	return nil
}

// RestoreCurrentStateFromFile loads the current context from a file
func (ec *ExecutionContext) RestoreCurrentStateFromFile() error {
	ec.m.Lock()
	defer ec.m.Unlock()
	filepath := ec.getPersistedStateFilePath()
	file, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}
	var restoredExecutionContextState State
	err = json.Unmarshal(file, &restoredExecutionContextState)
	if err != nil {
		return err
	}
	ec.arn = restoredExecutionContextState.ARN
	ec.lastRequestID = restoredExecutionContextState.LastRequestID
	ec.lastLogRequestID = restoredExecutionContextState.LastLogRequestID
	ec.lastOOMRequestID = restoredExecutionContextState.LastOOMRequestID
	ec.coldstartRequestID = restoredExecutionContextState.ColdstartRequestID
	ec.startTime = restoredExecutionContextState.StartTime
	ec.endTime = restoredExecutionContextState.EndTime
	ec.isStateSaved = false
	return nil
}

// IsStateSaved returns whether the state has been saved in the current execution
func (ec *ExecutionContext) IsStateSaved() bool {
	ec.m.Lock()
	defer ec.m.Unlock()
	return ec.isStateSaved
}
