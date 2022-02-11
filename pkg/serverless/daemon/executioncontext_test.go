// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetExecutionContextFromInvocationUppercase(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()
	testArn := "arn:aws:lambda:us-east-1:123456789012:function:MY-SUPER-function"
	testRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	d.SetExecutionContextFromInvocation(testArn, testRequestID)
	assert.Equal("arn:aws:lambda:us-east-1:123456789012:function:my-super-function", d.executionContext.ARN)
	assert.Equal(testRequestID, d.executionContext.LastRequestID)
	assert.Equal(true, d.executionContext.Coldstart)
	assert.Equal(testRequestID, d.executionContext.ColdstartRequestID)
}

func TestSetExecutionContextFromInvocationNoColdstart(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()
	d.executionContext.ColdstartRequestID = "coldstart-request-id"
	testArn := "arn:aws:lambda:us-east-1:123456789012:function:MY-SUPER-function"
	testRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	d.SetExecutionContextFromInvocation(testArn, testRequestID)
	assert.Equal("arn:aws:lambda:us-east-1:123456789012:function:my-super-function", d.executionContext.ARN)
	assert.Equal(testRequestID, d.executionContext.LastRequestID)
	assert.Equal(false, d.executionContext.Coldstart)
}
