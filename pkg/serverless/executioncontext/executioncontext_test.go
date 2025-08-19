// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package executioncontext

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetCurrentState(t *testing.T) {
	assert := assert.New(t)

	testArn := "arn:aws:lambda:us-east-1:123456789012:function:MY-SUPER-function"
	testRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	ec := ExecutionContext{}
	ec.initTime = time.Now()
	ec.SetFromInvocation(testArn, testRequestID)

	ecs := ec.GetCurrentState()
	coldStartTags := ec.GetColdStartTagsForRequestID(ecs.LastRequestID)
	assert.Equal(testRequestID, ecs.LastRequestID)
	assert.Equal(true, coldStartTags.IsColdStart)
	assert.Equal(testRequestID, ecs.ColdstartRequestID)
}

func TestSetFromInvocationUppercase(t *testing.T) {
	assert := assert.New(t)

	testArn := "arn:aws:lambda:us-east-1:123456789012:function:MY-SUPER-function"
	testRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	ec := ExecutionContext{}
	ec.initTime = time.Now()
	ec.SetFromInvocation(testArn, testRequestID)
	coldStartTags := ec.GetColdStartTagsForRequestID(ec.lastRequestID)
	assert.Equal("arn:aws:lambda:us-east-1:123456789012:function:my-super-function", ec.arn)
	assert.Equal(testRequestID, ec.lastRequestID)
	assert.Equal(true, coldStartTags.IsColdStart)
	assert.Equal(testRequestID, ec.coldstartRequestID)
}

func TestSetFromInvocationWarmStart(t *testing.T) {
	assert := assert.New(t)

	testArn := "arn:aws:lambda:us-east-1:123456789012:function:MY-SUPER-function"
	testRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"

	ec := ExecutionContext{}
	ec.SetFromInvocation(testArn, "coldstart-request-id")
	ec.SetFromInvocation(testArn, testRequestID)

	coldStartTags := ec.GetColdStartTagsForRequestID(ec.lastRequestID)

	assert.Equal("arn:aws:lambda:us-east-1:123456789012:function:my-super-function", ec.arn)
	assert.Equal(testRequestID, ec.lastRequestID)
	assert.Equal(false, coldStartTags.IsColdStart)
}

func TestSetArnFromExtensionResponse(t *testing.T) {
	assert := assert.New(t)

	testArn := "arn:aws:lambda:us-east-1:123456789012:function:MY-SUPER-function"

	ec := ExecutionContext{}
	ec.SetArnFromExtensionResponse(testArn)

	assert.Equal("arn:aws:lambda:us-east-1:123456789012:function:my-super-function", ec.arn)
}

func TestUpdateFromStartLog(t *testing.T) {
	assert := assert.New(t)

	startTime := time.Now()
	ec := ExecutionContext{}
	ec.UpdateStartTime(startTime)

	assert.Equal(startTime, ec.startTime)
}

func TestSaveAndRestoreFromFile(t *testing.T) {
	assert := assert.New(t)

	tempfile, err := os.CreateTemp("/tmp", "dd-lambda-extension-cache-*.json")
	assert.Nil(err)
	defer os.Remove(tempfile.Name())

	testArn := "arn:aws:lambda:us-east-1:123456789012:function:my-super-function"
	testRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	startTime := time.Now()
	endTime := startTime.Add(10 * time.Second)
	ec := ExecutionContext{persistedStateFilePath: tempfile.Name()}
	ec.SetFromInvocation(testArn, testRequestID)
	ec.UpdateOutOfMemoryRequestID(testRequestID)
	ec.UpdateStartTime(startTime)
	ec.UpdateEndTime(endTime)

	err = ec.SaveCurrentExecutionContext()
	assert.Nil(err)

	ec.UpdateStartTime(startTime.Add(time.Hour))
	ec.UpdateEndTime(endTime.Add(time.Hour))
	ec.SetFromInvocation("this-arn-should-be-overwritten", "this-request-id-should-be-overwritten")

	err = ec.RestoreCurrentStateFromFile()
	assert.Nil(err)

	assert.Equal(testRequestID, ec.lastRequestID)
	assert.Equal(testRequestID, ec.lastOOMRequestID)
	assert.Equal(testArn, ec.arn)
	assert.WithinDuration(startTime, ec.startTime, time.Millisecond)
	assert.WithinDuration(endTime, ec.endTime, time.Millisecond)
}

func TestUpdateFromRuntimeDoneLog(t *testing.T) {
	assert := assert.New(t)

	endTime := time.Now()
	ec := ExecutionContext{}
	ec.UpdateEndTime(endTime)

	assert.Equal(endTime, ec.endTime)
}

func TestUpdateOutOfMemoryRequestID(t *testing.T) {
	testRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	ec := ExecutionContext{}
	ec.UpdateOutOfMemoryRequestID(testRequestID)

	ecs := ec.GetCurrentState()
	assert.Equal(t, testRequestID, ecs.LastOOMRequestID)
}

func TestUpdateRuntime(t *testing.T) {
	runtime := "dotnet6"
	ec := ExecutionContext{}
	ec.UpdateRuntime(runtime)

	ecs := ec.GetCurrentState()
	assert.Equal(t, ecs.Runtime, runtime)
}

func TestIsStateSaved(t *testing.T) {
	tempfile, err := os.CreateTemp(t.TempDir(), "dd-lambda-extension-cache-*.json")
	assert.Nil(t, err)
	defer os.Remove(tempfile.Name())

	ec := ExecutionContext{persistedStateFilePath: tempfile.Name()}
	assert.False(t, ec.IsStateSaved())
	err = ec.SaveCurrentExecutionContext()
	assert.Nil(t, err)
	assert.True(t, ec.IsStateSaved())

	err = ec.RestoreCurrentStateFromFile()
	assert.Nil(t, err)
	assert.Equal(t, ec.isStateSaved, ec.IsStateSaved())
}

func TestUpdatePersistedStateFilePath(t *testing.T) {
	ec := ExecutionContext{}
	assert.Equal(t, "", ec.persistedStateFilePath)

	ec.UpdatePersistedStateFilePath("test-file-path")
	assert.Equal(t, "test-file-path", ec.persistedStateFilePath)
}
