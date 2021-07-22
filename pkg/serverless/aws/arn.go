// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aws

import (
	"encoding/json"
	"io/ioutil"
	"strings"
	"sync"
)

const (
	persistedStateFilePath = "/tmp/dd-lambda-extension-cache.json"
)

type persistedState struct {
	CurrentARN   string
	CurrentReqID string
}

var currentARN struct {
	value     string
	qualifier string
	sync.Mutex
}

var currentReqID struct {
	value string
	sync.Mutex
}

var currentColdStart struct {
	value bool
	sync.Mutex
}

// GetARN returns an ARN of the current running function.
// Thread-safe.
func GetARN() string {
	currentARN.Lock()
	defer currentARN.Unlock()

	return currentARN.value
}

// GetQualifier returns the qualifier for the current running function.
// Thread-safe
func GetQualifier() string {
	currentARN.Lock()
	defer currentARN.Unlock()
	return currentARN.qualifier
}

// GetColdStart returns whether the current invocation is a cold start
// Thread-safe
func GetColdStart() bool {
	currentColdStart.Lock()
	defer currentColdStart.Unlock()
	return currentColdStart.value
}

// SetARN stores the given ARN.
// Thread-safe.
func SetARN(arn string) {
	currentARN.Lock()
	defer currentARN.Unlock()

	arn = strings.ToLower(arn)

	qualifier := ""
	// remove the version if any
	if parts := strings.Split(arn, ":"); len(parts) > 7 {
		arn = strings.Join(parts[:7], ":")
		qualifier = strings.TrimPrefix(parts[7], "$")
	}

	currentARN.value = arn
	currentARN.qualifier = qualifier
}

// GetRequestID returns the currently running function request ID.
func GetRequestID() string {
	currentReqID.Lock()
	defer currentReqID.Unlock()

	return currentReqID.value
}

// SetRequestID stores the currently running function request ID.
func SetRequestID(reqID string) {
	currentReqID.Lock()
	defer currentReqID.Unlock()

	currentReqID.value = reqID
}

// SetColdStart stores the cold start state of the function
func SetColdStart(coldstart bool) {
	currentColdStart.Lock()
	defer currentColdStart.Unlock()

	currentColdStart.value = coldstart
}

// PersistCurrentStateToFile persists the current state (ARN and Request ID) to a file.
// This allows the state to be restored after the extension restarts.
// Call this function when the extension shuts down.
func PersistCurrentStateToFile() error {
	dataToPersist := persistedState{
		CurrentARN:   GetARN(),
		CurrentReqID: GetRequestID(),
	}

	file, err := json.MarshalIndent(dataToPersist, "", "")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(persistedStateFilePath, file, 0644)
	if err != nil {
		return err
	}
	return nil
}

// RestoreCurrentStateFromFile restores the current state (ARN and Request ID) from a file
// after the extension is restarted. Call this function when the extension starts.
func RestoreCurrentStateFromFile() error {
	file, err := ioutil.ReadFile(persistedStateFilePath)
	if err != nil {
		return err
	}
	var restoredState persistedState
	err = json.Unmarshal(file, &restoredState)
	if err != nil {
		return err
	}
	SetARN(restoredState.CurrentARN)
	SetRequestID(restoredState.CurrentReqID)
	return nil
}
