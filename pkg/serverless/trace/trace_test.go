// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package trace

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

func TestStartEnabledFalse(t *testing.T) {
	var agent = &ServerlessTraceAgent{}
	agent.Start(false, nil)
	defer agent.Stop()
	assert.Nil(t, agent.ta)
	assert.Nil(t, agent.Get())
	assert.Nil(t, agent.cancel)
}

type LoadConfigMocked struct {
	Path string
}

func (l *LoadConfigMocked) Load() (*config.AgentConfig, error) {
	return nil, fmt.Errorf("error")
}

func TestStartEnabledTrueInvalidConfig(t *testing.T) {
	var agent = &ServerlessTraceAgent{}
	agent.Start(true, &LoadConfigMocked{})
	defer agent.Stop()
	assert.Nil(t, agent.ta)
	assert.Nil(t, agent.Get())
	assert.Nil(t, agent.cancel)
}

func TestStartEnabledTrueValidConfigUnvalidPath(t *testing.T) {
	var agent = &ServerlessTraceAgent{}

	t.Setenv("DD_API_KEY", "x")
	agent.Start(true, &LoadConfig{Path: "invalid.yml"})
	defer agent.Stop()
	assert.NotNil(t, agent.ta)
	assert.NotNil(t, agent.Get())
	assert.NotNil(t, agent.cancel)
}

func TestStartEnabledTrueValidConfigValidPath(t *testing.T) {
	var agent = &ServerlessTraceAgent{}

	agent.Start(true, &LoadConfig{Path: "./testdata/valid.yml"})
	defer agent.Stop()
	assert.NotNil(t, agent.ta)
	assert.NotNil(t, agent.Get())
	assert.NotNil(t, agent.cancel)
}

func TestLoadConfigShouldBeFast(t *testing.T) {
	startTime := time.Now()
	agent := &ServerlessTraceAgent{}
	agent.Start(true, &LoadConfig{Path: "./testdata/valid.yml"})
	defer agent.Stop()
	assert.True(t, time.Since(startTime) < time.Second)
}

func TestFilterSpanFromLambdaLibraryOrRuntime(t *testing.T) {
	spanFromLambdaLibrary := pb.Span{
		Meta: map[string]string{
			"http.url": "http://127.0.0.1:8124/lambda/flush",
		},
	}

	spanFromLambdaRuntime := pb.Span{
		Meta: map[string]string{
			"http.url": "http://127.0.0.1:9001/2018-06-01/runtime/invocation/fee394a9-b9a4-4602-853e-a48bb663caa3/response",
		},
	}

	spanFromStatsD := pb.Span{
		Meta: map[string]string{
			"http.url": "http://127.0.0.1:8125/",
		},
	}

	legitimateSpan := pb.Span{
		Meta: map[string]string{
			"http.url": "http://www.datadoghq.com",
		},
	}

	assert.True(t, filterSpanFromLambdaLibraryOrRuntime(&spanFromLambdaLibrary))
	assert.True(t, filterSpanFromLambdaLibraryOrRuntime(&spanFromLambdaRuntime))
	assert.True(t, filterSpanFromLambdaLibraryOrRuntime(&spanFromStatsD))
	assert.False(t, filterSpanFromLambdaLibraryOrRuntime(&legitimateSpan))
}
