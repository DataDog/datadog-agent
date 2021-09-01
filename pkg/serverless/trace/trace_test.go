// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !windows

package trace

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
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

	os.Setenv("DD_API_KEY", "x")
	defer os.Unsetenv("DD_API_KEY")
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
	timeout := time.After(1 * time.Second)
	done := make(chan bool)
	go func() {
		var agent = &ServerlessTraceAgent{}
		agent.Start(true, &LoadConfig{Path: "./testdata/valid.yml"})
		defer agent.Stop()
		done <- true
	}()

	select {
	case <-timeout:
		t.Fatal("Tracer config load/validation is too long")
	case <-done:
	}
}
