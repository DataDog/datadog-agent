// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cast"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func setupTest() {
	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	config.InitConfig(config.Datadog)
}

func TestTagsSetup(t *testing.T) {
	// TODO: Fix and re-enable flaky test
	t.Skip()
	setupTest()

	ddTagsEnv := "key1:value1 key2:value2 key3:value3:4"
	ddExtraTagsEnv := "key22:value22 key23:value23"
	t.Setenv("DD_TAGS", ddTagsEnv)
	t.Setenv("DD_EXTRA_TAGS", ddExtraTagsEnv)
	ddTags := cast.ToStringSlice(ddTagsEnv)
	ddExtraTags := cast.ToStringSlice(ddExtraTagsEnv)

	allTags := append(ddTags, ddExtraTags...)

	_, _, traceAgent, metricAgent, _ := setup("", nil, nil)
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

func TestFlushSucess(t *testing.T) {
	metricAgent := &TestFlushableAgent{}
	traceAgent := &TestFlushableAgent{}
	mockLogsAgent := logsAgent.NewMockServerlessLogsAgent()
	lastFlush(100*time.Millisecond, metricAgent, traceAgent, mockLogsAgent)
	assert.Equal(t, true, metricAgent.hasBeenCalled)
	assert.Equal(t, true, mockLogsAgent.DidFlush())
}

func TestFlushTimeout(t *testing.T) {
	metricAgent := &TestTimeoutFlushableAgent{}
	traceAgent := &TestTimeoutFlushableAgent{}
	mockLogsAgent := logsAgent.NewMockServerlessLogsAgent()
	mockLogsAgent.SetFlushDelay(time.Hour)

	lastFlush(100*time.Millisecond, metricAgent, traceAgent, mockLogsAgent)
	assert.Equal(t, false, metricAgent.hasBeenCalled)
	assert.Equal(t, false, mockLogsAgent.DidFlush())
}
