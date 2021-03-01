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

const persistedStateFilePath = "/tmp/dd-lambda-extension-cache.json"

type persistedState struct {
	CurrentARN   string
	CurrentReqID string
}

var currentARN struct {
	value string
	sync.Mutex
}

var currentReqID struct {
	value string
	sync.Mutex
}

// GetARN returns an ARN of the current running function.
// Thread-safe.
func GetARN() string {
	currentARN.Lock()
	defer currentARN.Unlock()

	return currentARN.value
}

// SetARN stores the given ARN.
// Thread-safe.
func SetARN(arn string) {
	currentARN.Lock()
	defer currentARN.Unlock()

	arn = strings.ToLower(arn)

	// remove the version if any
	// format: arn:aws:lambda:<region>:<account-id>:function:<function-name>[:<version>]
	if parts := strings.Split(arn, ":"); len(parts) > 7 {
		arn = strings.Join(parts[:7], ":")
	}

	currentARN.value = arn
}

// FunctionNameFromARN returns the function name from the currently set ARN.
// Thread-safe.
func FunctionNameFromARN() string {
	arn := GetARN()
	parts := strings.Split(arn, ":")
	return parts[len(parts)-1]
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
