// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/serverless/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
)

func TestSetTraceTagNoop(t *testing.T) {
	tagsMap := map[string]string{
		"key0": "value0",
	}
	d := Daemon{
		TraceAgent: nil,
	}
	assert.False(t, d.setTraceTags(tagsMap))
}

func TestSetTraceTagNoopTraceGetNil(t *testing.T) {
	tagsMap := map[string]string{
		"key0": "value0",
	}
	d := Daemon{
		TraceAgent: &trace.ServerlessTraceAgent{},
	}
	assert.False(t, d.setTraceTags(tagsMap))
}

func TestSetTraceTagOk(t *testing.T) {
	tagsMap := map[string]string{
		"key0": "value0",
	}
	var agent = &trace.ServerlessTraceAgent{}
	os.Setenv("DD_API_KEY", "x")
	defer os.Unsetenv("DD_API_KEY")
	orchestrator := orchestrator.NewLambdaOrchestrator()
	agent.Start(true, &trace.LoadConfig{Path: "/does-not-exist.yml"}, orchestrator)
	defer agent.Stop()
	d := Daemon{
		TraceAgent: agent,
	}
	assert.True(t, d.setTraceTags(tagsMap))
}
