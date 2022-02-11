// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"encoding/json"
	"io/ioutil"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/executioncontext"
)

// GetExecutionContext gets the current execution context
func (d *Daemon) GetExecutionContext() executioncontext.ExecutionContext {
	d.executionContextMutex.Lock()
	defer d.executionContextMutex.Unlock()
	return *d.executionContext
}

// SetExecutionContextFromInvocation sets the execution context based on an invocation
func (d *Daemon) SetExecutionContextFromInvocation(arn string, requestID string) {
	d.executionContextMutex.Lock()
	defer d.executionContextMutex.Unlock()
	d.executionContext.ARN = strings.ToLower(arn)
	d.executionContext.LastRequestID = requestID
	if len(d.executionContext.ColdstartRequestID) == 0 {
		d.executionContext.Coldstart = true
		d.executionContext.ColdstartRequestID = requestID
	} else {
		d.executionContext.Coldstart = false
	}
}

// UpdateExecutionContextFromStartLog updates the execution context based on a platform.Start log message
func (d *Daemon) UpdateExecutionContextFromStartLog(requestID string, time time.Time) {
	d.executionContextMutex.Lock()
	defer d.executionContextMutex.Unlock()
	d.executionContext.LastLogRequestID = requestID
	d.executionContext.StartTime = time
}

// SaveCurrentExecutionContext stores the current context to a file
func (d *Daemon) SaveCurrentExecutionContext() error {
	d.executionContextMutex.Lock()
	defer d.executionContextMutex.Unlock()
	file, err := json.Marshal(d.executionContext)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(persistedStateFilePath, file, 0644)
	if err != nil {
		return err
	}
	return nil
}

// RestoreCurrentStateFromFile loads the current context from a file
func (d *Daemon) RestoreCurrentStateFromFile() error {
	d.executionContextMutex.Lock()
	defer d.executionContextMutex.Unlock()
	file, err := ioutil.ReadFile(persistedStateFilePath)
	if err != nil {
		return err
	}
	var restoredExecutionContext executioncontext.ExecutionContext
	err = json.Unmarshal(file, &restoredExecutionContext)
	if err != nil {
		return err
	}
	d.executionContext.ARN = restoredExecutionContext.ARN
	d.executionContext.LastRequestID = restoredExecutionContext.LastRequestID
	d.executionContext.LastLogRequestID = restoredExecutionContext.LastLogRequestID
	d.executionContext.ColdstartRequestID = restoredExecutionContext.ColdstartRequestID
	d.executionContext.StartTime = restoredExecutionContext.StartTime
	return nil
}
