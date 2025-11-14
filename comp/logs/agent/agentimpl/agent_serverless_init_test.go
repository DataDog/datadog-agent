// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package agentimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	compressionmock "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
)

func TestBuildServerlessEndpoints(t *testing.T) {
	config := config.NewMock(t)

	endpoints, err := buildEndpoints(config)
	assert.Nil(t, err)
	assert.Equal(t, "http-intake.logs.datadoghq.com.", endpoints.Main.Host)
	assert.Equal(t, "serverless", string(endpoints.Main.Origin))
}

func TestServerlessLogsAgent(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeCompression := compressionmock.NewMockCompressor()
	hostnameService := hostnameimpl.NewHostnameService()
	config := config.NewMock(t)

	serverlessLogsAgent := NewServerlessLogsAgent(fakeTagger, fakeCompression, hostnameService)
	logsAgent, ok := serverlessLogsAgent.(*logAgent)
	assert.True(t, ok, "Expected NewServerlessLogsAgent to return *logAgent type")

	// Build endpoints first, setupAgent() calls SetupPipeline which references logsAgent.endpoints
	endpoints, err := buildEndpoints(config)
	assert.NoError(t, err, "buildEndpoints should not fail")
	logsAgent.endpoints = endpoints

	err = logsAgent.setupAgent()
	assert.NoError(t, err, "setupAgent should not return an error")

	// Assert that setupAgent() correctly initialized the pipeline components
	assert.NotNil(t, logsAgent.diagnosticMessageReceiver, "diagnosticMessageReceiver should not be nil")
	assert.NotNil(t, logsAgent.pipelineProvider, "pipelineProvider should be set")
	assert.NotNil(t, logsAgent.launchers, "launchers should be set")
	assert.NotNil(t, logsAgent.destinationsCtx, "destinationsCtx should be set")
	assert.NotNil(t, logsAgent.schedulers, "schedulers should be set")

	// Assert that the hostname component returns an empty string
	hostname, err := logsAgent.hostname.Get(context.TODO())
	assert.NoError(t, err, "Getting hostname should not error")
	assert.Equal(t, "", hostname, "hostname should be an empty string")
}
