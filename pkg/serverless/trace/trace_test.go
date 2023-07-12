// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package trace

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/serverless/random"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

func setupTraceAgentTest(t *testing.T) {
	// ensure a free port is used for starting the trace agent
	if port, err := testutil.FindTCPPort(); err == nil {
		t.Setenv("DD_RECEIVER_PORT", strconv.Itoa(port))
	}
}

func TestStartEnabledFalse(t *testing.T) {
	setupTraceAgentTest(t)

	lambdaSpanChan := make(chan *pb.Span)
	var agent = &ServerlessTraceAgent{}
	agent.Start(false, nil, lambdaSpanChan, random.Random.Uint64())
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
	setupTraceAgentTest(t)

	var agent = &ServerlessTraceAgent{}
	lambdaSpanChan := make(chan *pb.Span)
	agent.Start(true, &LoadConfigMocked{}, lambdaSpanChan, random.Random.Uint64())
	defer agent.Stop()
	assert.Nil(t, agent.ta)
	assert.Nil(t, agent.Get())
	assert.Nil(t, agent.cancel)
}

func TestStartEnabledTrueValidConfigUnvalidPath(t *testing.T) {
	setupTraceAgentTest(t)

	var agent = &ServerlessTraceAgent{}
	lambdaSpanChan := make(chan *pb.Span)

	t.Setenv("DD_API_KEY", "x")
	agent.Start(true, &LoadConfig{Path: "invalid.yml"}, lambdaSpanChan, random.Random.Uint64())
	defer agent.Stop()
	assert.NotNil(t, agent.ta)
	assert.NotNil(t, agent.Get())
	assert.NotNil(t, agent.cancel)
}

func TestStartEnabledTrueValidConfigValidPath(t *testing.T) {
	setupTraceAgentTest(t)

	var agent = &ServerlessTraceAgent{}
	lambdaSpanChan := make(chan *pb.Span)

	agent.Start(true, &LoadConfig{Path: "./testdata/valid.yml"}, lambdaSpanChan, random.Random.Uint64())
	defer agent.Stop()
	assert.NotNil(t, agent.ta)
	assert.NotNil(t, agent.Get())
	assert.NotNil(t, agent.cancel)
}

func TestLoadConfigShouldBeFast(t *testing.T) {
	setupTraceAgentTest(t)

	startTime := time.Now()
	lambdaSpanChan := make(chan *pb.Span)

	agent := &ServerlessTraceAgent{}
	agent.Start(true, &LoadConfig{Path: "./testdata/valid.yml"}, lambdaSpanChan, random.Random.Uint64())
	defer agent.Stop()
	assert.True(t, time.Since(startTime) < time.Second)
}

func TestFilterSpanFromLambdaLibraryOrRuntimeHttpSpan(t *testing.T) {
	httpSpanFromLambdaLibrary := pb.Span{
		Meta: map[string]string{
			"http.url": "http://127.0.0.1:8124/lambda/flush",
		},
	}

	httpSpanFromLambdaRuntime := pb.Span{
		Meta: map[string]string{
			"http.url": "http://127.0.0.1:9001/2018-06-01/runtime/invocation/fee394a9-b9a4-4602-853e-a48bb663caa3/response",
		},
	}

	httpSpanFromStatsD := pb.Span{
		Meta: map[string]string{
			"http.url": "http://127.0.0.1:8125/",
		},
	}
	assert.True(t, filterSpanFromLambdaLibraryOrRuntime(&httpSpanFromLambdaLibrary))
	assert.True(t, filterSpanFromLambdaLibraryOrRuntime(&httpSpanFromLambdaRuntime))
	assert.True(t, filterSpanFromLambdaLibraryOrRuntime(&httpSpanFromStatsD))
}

func TestFilterSpanFromLambdaLibraryOrRuntimeTcpSpan(t *testing.T) {
	tcpSpanFromLambdaLibrary := pb.Span{
		Meta: map[string]string{
			"tcp.remote.host": "127.0.0.1",
			"tcp.remote.port": "8124",
		},
	}

	tcpSpanFromLambdaRuntime := pb.Span{
		Meta: map[string]string{
			"tcp.remote.host": "127.0.0.1",
			"tcp.remote.port": "9001",
		},
	}

	tcpSpanFromStatsD := pb.Span{
		Meta: map[string]string{
			"tcp.remote.host": "127.0.0.1",
			"tcp.remote.port": "8125",
		},
	}
	assert.True(t, filterSpanFromLambdaLibraryOrRuntime(&tcpSpanFromLambdaLibrary))
	assert.True(t, filterSpanFromLambdaLibraryOrRuntime(&tcpSpanFromLambdaRuntime))
	assert.True(t, filterSpanFromLambdaLibraryOrRuntime(&tcpSpanFromStatsD))
}

func TestFilterSpanFromLambdaLibraryOrRuntimeDnsSpan(t *testing.T) {
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
	assert.True(t, filterSpanFromLambdaLibraryOrRuntime(&dnsSpanFromLocalhostAddress))
	assert.True(t, filterSpanFromLambdaLibraryOrRuntime(&dnsSpanFromNonRoutableAddress))
}

func TestFilterSpanFromLambdaLibraryOrRuntimeLegitimateSpan(t *testing.T) {
	legitimateSpan := pb.Span{
		Meta: map[string]string{
			"http.url": "http://www.datadoghq.com",
		},
	}
	assert.False(t, filterSpanFromLambdaLibraryOrRuntime(&legitimateSpan))
}

func TestFilterServerlessSpanFromTracer(t *testing.T) {
	span := pb.Span{
		Resource: invocationSpanResource,
	}
	assert.True(t, filterSpanFromLambdaLibraryOrRuntime(&span))
}
