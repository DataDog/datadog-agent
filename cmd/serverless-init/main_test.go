// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/spf13/cast"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/logs/agent/agentimpl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestTagsSetup(t *testing.T) {
	// TODO: Fix and re-enable flaky test
	t.Skip()

	fakeTagger := taggerimpl.SetupFakeTagger(t)

	configmock.New(t)

	ddTagsEnv := "key1:value1 key2:value2 key3:value3:4"
	ddExtraTagsEnv := "key22:value22 key23:value23"
	t.Setenv("DD_TAGS", ddTagsEnv)
	t.Setenv("DD_EXTRA_TAGS", ddExtraTagsEnv)
	ddTags := cast.ToStringSlice(ddTagsEnv)
	ddExtraTags := cast.ToStringSlice(ddExtraTagsEnv)

	allTags := append(ddTags, ddExtraTags...)

	_, _, traceAgent, metricAgent, _ := setup(mode.Conf{}, fakeTagger)
	defer traceAgent.Stop()
	defer metricAgent.Stop()
	assert.Subset(t, metricAgent.GetExtraTags(), allTags)
	assert.Subset(t, logs.GetLogsTags(), allTags)
}

func TestFxApp(t *testing.T) {
	fxutil.TestOneShot(t, main)
}

type TestTimeoutFlushableAgent struct {
	hasBeenCalled bool
}

type TestFlushableAgent struct {
	hasBeenCalled bool
}

func (tfa *TestTimeoutFlushableAgent) Flush() {
	time.Sleep(1 * time.Hour)
	tfa.hasBeenCalled = true
}

func (tfa *TestFlushableAgent) Flush() {
	tfa.hasBeenCalled = true
}

func TestFlushSuccess(t *testing.T) {
	metricAgent := &TestFlushableAgent{}
	traceAgent := &TestFlushableAgent{}
	mockLogsAgent := agentimpl.NewMockServerlessLogsAgent()
	lastFlush(100*time.Millisecond, metricAgent, traceAgent, mockLogsAgent)
	assert.Equal(t, true, metricAgent.hasBeenCalled)
	assert.Equal(t, true, mockLogsAgent.DidFlush())
}

func TestFlushTimeout(t *testing.T) {
	metricAgent := &TestTimeoutFlushableAgent{}
	traceAgent := &TestTimeoutFlushableAgent{}
	mockLogsAgent := agentimpl.NewMockServerlessLogsAgent()
	mockLogsAgent.SetFlushDelay(time.Hour)

	lastFlush(100*time.Millisecond, metricAgent, traceAgent, mockLogsAgent)
	assert.Equal(t, false, metricAgent.hasBeenCalled)
	assert.Equal(t, false, mockLogsAgent.DidFlush())
}
func TestExitCodePropagationGenericError(t *testing.T) {
	err := errors.New("test error")

	exitCode := errorExitCode(err)
	assert.Equal(t, 1, exitCode)
}

func TestExitCodePropagationExitError(t *testing.T) {
	cmd := exec.Command("bash", "-c", "exit 2")
	err := cmd.Run()

	exitCode := errorExitCode(err)
	assert.Equal(t, 2, exitCode)
}

func TestExitCodePropagationJoinedExitError(t *testing.T) {
	genericError := errors.New("test error")

	cmd := exec.Command("bash", "-c", "exit 3")
	exitCodeError := cmd.Run()

	errs := errors.Join(genericError, exitCodeError)

	exitCode := errorExitCode(errs)
	assert.Equal(t, 3, exitCode)
}
