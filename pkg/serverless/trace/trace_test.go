// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package trace

import (
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

func setupTraceAgentTest(t *testing.T) {
	// ensure a free port is used for starting the trace agent
	if port, err := testutil.FindTCPPort(); err == nil {
		t.Setenv("DD_RECEIVER_PORT", strconv.Itoa(port))
	}
}

type LoadConfigMocked struct {
	Path string
}

func (l *LoadConfigMocked) Load() (*config.AgentConfig, error) {
	return nil, errors.New("error")
}

func TestStartEnabledTrueInvalidConfig(t *testing.T) {
	setupTraceAgentTest(t)

	agent := StartServerlessTraceAgent(StartServerlessTraceAgentArgs{
		Enabled:    true,
		LoadConfig: &LoadConfigMocked{},
	})
	defer agent.Stop()
	assert.NotNil(t, agent)
	assert.IsType(t, noopTraceAgent{}, agent)
}

func TestStartEnabledTrueValidConfigInvalidPath(t *testing.T) {
	setupTraceAgentTest(t)

	configmock.SetDefaultConfigType(t, "yaml")
	t.Setenv("DD_API_KEY", "x")
	agent := StartServerlessTraceAgent(StartServerlessTraceAgentArgs{
		Enabled:    true,
		LoadConfig: &LoadConfig{Path: "invalid.yml"},
	})
	defer agent.Stop()
	assert.NotNil(t, agent)
	assert.IsType(t, &serverlessTraceAgent{}, agent)
}

func TestStartEnabledTrueValidConfigValidPath(t *testing.T) {
	setupTraceAgentTest(t)

	agent := StartServerlessTraceAgent(StartServerlessTraceAgentArgs{
		Enabled:    true,
		LoadConfig: &LoadConfig{Path: "./testdata/valid.yml"},
	})
	defer agent.Stop()
	assert.NotNil(t, agent)
	assert.IsType(t, &serverlessTraceAgent{}, agent)
}

func TestFilterSpanFromRuntimeHttpSpan(t *testing.T) {
	httpSpanFromStatsD := pb.Span{
		Meta: map[string]string{
			"http.url": "http://127.0.0.1:8125/",
		},
	}
	assert.True(t, filterSpan(&httpSpanFromStatsD))
}

func TestFilterSpanFromRuntimeTcpSpan(t *testing.T) {
	tcpSpanFromStatsD := pb.Span{
		Meta: map[string]string{
			"tcp.remote.host": "127.0.0.1",
			"tcp.remote.port": "8125",
		},
	}
	assert.True(t, filterSpan(&tcpSpanFromStatsD))
}

func TestFilterSpanFromRuntimeDnsSpan(t *testing.T) {
	dnsSpanFromLocalhostAddress := pb.Span{
		Meta: map[string]string{
			"dns.address": "127.0.0.1",
		},
	}

	dnsSpanFromNonRoutableAddress := pb.Span{
		Meta: map[string]string{
			"dns.address": "0.0.0.0",
		},
	}

	assert.True(t, filterSpan(&dnsSpanFromLocalhostAddress))
	assert.True(t, filterSpan(&dnsSpanFromNonRoutableAddress))
}

func TestFilterSpanFromRuntimeLegitimateSpan(t *testing.T) {
	legitimateSpan := pb.Span{
		Meta: map[string]string{
			"http.url": "http://www.datadoghq.com",
		},
	}
	assert.False(t, filterSpan(&legitimateSpan))
}

func TestGetDDOriginCloudServices(t *testing.T) {
	serviceToEnvVar := map[string]string{
		"cloudrun":     cloudservice.ServiceNameEnvVar,
		"appservice":   cloudservice.WebsiteStack,
		"containerapp": cloudservice.ContainerAppNameEnvVar,
	}
	for service, envVar := range serviceToEnvVar {
		t.Setenv(envVar, "myService")
		assert.Equal(t, service, getDDOrigin())
		os.Unsetenv(envVar)
	}
}

func TestStartServerlessTraceAgentFunctionTags(t *testing.T) {
	tests := []struct {
		name         string
		functionTags string
	}{
		{
			name:         "with function tags",
			functionTags: "env:production,service:my-service,version:1.0",
		},
		{
			name:         "with empty function tags",
			functionTags: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTraceAgentTest(t)

			agent := StartServerlessTraceAgent(StartServerlessTraceAgentArgs{
				Enabled:      true,
				LoadConfig:   &LoadConfig{Path: "./testdata/valid.yml"},
				FunctionTags: tt.functionTags,
			})
			defer agent.Stop()

			assert.NotNil(t, agent)
			assert.IsType(t, &serverlessTraceAgent{}, agent)

			// Access the underlying agent to check TracerPayloadModifier
			serverlessAgent := agent.(*serverlessTraceAgent)
			assert.NotNil(t, serverlessAgent.ta.TracerPayloadModifier)
		})
	}
}

func TestServerlessTraceAgentDisableTraceStats(t *testing.T) {
	tests := []struct {
		name       string
		envValue   string
		expectNoop bool
	}{
		{
			name:       "trace stats enabled by default",
			envValue:   "",
			expectNoop: false,
		},
		{
			name:       "trace stats disabled with true",
			envValue:   "true",
			expectNoop: true,
		},
		{
			name:       "trace stats enabled with false",
			envValue:   "false",
			expectNoop: false,
		},
		{
			name:       "trace stats enabled with other value",
			envValue:   "yes",
			expectNoop: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTraceAgentTest(t)

			if tt.envValue != "" {
				t.Setenv(disableTraceStatsEnvVar, tt.envValue)
			}

			agent := StartServerlessTraceAgent(StartServerlessTraceAgentArgs{
				Enabled:    true,
				LoadConfig: &LoadConfig{Path: "./testdata/valid.yml"},
			})
			defer agent.Stop()

			assert.NotNil(t, agent)
			assert.IsType(t, &serverlessTraceAgent{}, agent)

			// Access the underlying agent to check concentrator type
			serverlessAgent := agent.(*serverlessTraceAgent)
			if tt.expectNoop {
				assert.IsType(t, &noopConcentrator{}, serverlessAgent.ta.Concentrator)
			} else {
				// Should not be noop concentrator
				assert.NotEqual(t, &noopConcentrator{}, serverlessAgent.ta.Concentrator)
			}
		})
	}
}
