// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package initcontainer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBuildCommandParamWithArgs(t *testing.T) {
	name, args := buildCommandParam([]string{"superCmd", "--verbose", "path", "-i", "."})
	assert.Equal(t, "superCmd", name)
	assert.Equal(t, []string{"--verbose", "path", "-i", "."}, args)
}

func TestBuildCommandParam(t *testing.T) {
	name, args := buildCommandParam([]string{"superCmd"})
	assert.Equal(t, "superCmd", name)
	assert.Equal(t, []string{}, args)
}

type TestFlushableAgent struct {
	hasBeenCalled bool
}

func (tfa *TestFlushableAgent) Flush() {
	time.Sleep(10 * time.Millisecond)
	tfa.hasBeenCalled = true
}

func TestFlushSuccess(t *testing.T) {
	metricAgent := &TestFlushableAgent{}
	traceAgent := &TestFlushableAgent{}
	flush(100*time.Millisecond, metricAgent, traceAgent)
	assert.Equal(t, true, metricAgent.hasBeenCalled)
	assert.Equal(t, true, traceAgent.hasBeenCalled)
}

func TestFlushTimeoutNonBlocking(t *testing.T) {
	metricAgent := &TestFlushableAgent{}
	traceAgent := &TestFlushableAgent{}
	flush(1*time.Millisecond, metricAgent, traceAgent)
	assert.Equal(t, false, metricAgent.hasBeenCalled)
	assert.Equal(t, false, traceAgent.hasBeenCalled)
}
