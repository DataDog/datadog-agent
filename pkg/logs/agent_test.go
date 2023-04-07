// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package logs

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/client/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/service"

	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

type AgentTestSuite struct {
	suite.Suite
	testDir     string
	testLogFile string
	fakeLogs    int64

	source *sources.LogSource
}

func (suite *AgentTestSuite) SetupTest() {
	mockConfig := coreConfig.Mock(nil)

	var err error

	suite.testDir = suite.T().TempDir()

	suite.testLogFile = fmt.Sprintf("%s/test.log", suite.testDir)
	fd, err := os.Create(suite.testLogFile)
	suite.NoError(err)

	fd.WriteString("test log1\n test log2\n")
	suite.fakeLogs = 2 // Two lines.
	fd.Close()

	logConfig := config.LogsConfig{
		Type:       config.FileType,
		Path:       suite.testLogFile,
		Identifier: "test", // As it was from service-discovery to force the tailer to read from the start.
	}
	suite.source = sources.NewLogSource("", &logConfig)

	mockConfig.Set("logs_config.run_path", suite.testDir)
	// Shorter grace period for tests.
	mockConfig.Set("logs_config.stop_grace_period", 1)
}

func (suite *AgentTestSuite) TearDownTest() {
	// Resets the metrics we check.
	metrics.LogsDecoded.Set(0)
	metrics.LogsProcessed.Set(0)
	metrics.LogsSent.Set(0)
	metrics.DestinationErrors.Set(0)
	metrics.DestinationLogsDropped.Init()
}

func createAgent(endpoints *config.Endpoints) (*Agent, *sources.LogSources, *service.Services) {
	// setup the sources and the services
	sources := sources.NewLogSources()
	services := service.NewServices()

	// setup and start the agent
	agent = NewAgent(sources, services, nil, endpoints)
	return agent, sources, services
}

func (suite *AgentTestSuite) testAgent(endpoints *config.Endpoints) {
	coreConfig.SetFeatures(suite.T(), coreConfig.Docker, coreConfig.Kubernetes)

	agent, sources, _ := createAgent(endpoints)

	zero := int64(0)
	assert.Equal(suite.T(), zero, metrics.LogsDecoded.Value())
	assert.Equal(suite.T(), zero, metrics.LogsProcessed.Value())
	assert.Equal(suite.T(), zero, metrics.LogsSent.Value())
	assert.Equal(suite.T(), zero, metrics.DestinationErrors.Value())
	assert.Equal(suite.T(), "{}", metrics.DestinationLogsDropped.String())

	agent.Start()
	sources.AddSource(suite.source)
	// Give the agent at most 4 second to send the logs. (seems to be slow on Windows/AppVeyor)
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 4*time.Second, func() bool {
		return suite.fakeLogs == metrics.LogsSent.Value()
	})
	agent.Stop()

	assert.Equal(suite.T(), suite.fakeLogs, metrics.LogsDecoded.Value())
	assert.Equal(suite.T(), suite.fakeLogs, metrics.LogsProcessed.Value())
	assert.Equal(suite.T(), suite.fakeLogs, metrics.LogsSent.Value())
	assert.Equal(suite.T(), zero, metrics.DestinationErrors.Value())
}

func (suite *AgentTestSuite) TestAgentTcp() {
	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()

	endpoint := tcp.AddrToEndPoint(l.Addr())
	endpoints := config.NewEndpoints(endpoint, nil, true, false)

	suite.testAgent(endpoints)
}

func (suite *AgentTestSuite) TestAgentHttp() {
	server := http.NewTestServer(200)
	defer server.Stop()
	endpoints := config.NewEndpoints(server.Endpoint, nil, false, true)

	suite.testAgent(endpoints)
}

func (suite *AgentTestSuite) TestAgentStopsWithWrongBackendTcp() {
	endpoint := config.Endpoint{Host: "fake:", Port: 0}
	endpoints := config.NewEndpoints(endpoint, []config.Endpoint{}, true, false)

	coreConfig.SetFeatures(suite.T(), coreConfig.Docker, coreConfig.Kubernetes)

	agent, sources, _ := createAgent(endpoints)

	agent.Start()
	sources.AddSource(suite.source)
	// Give the agent at most one second to process the logs.
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 2*time.Second, func() bool {
		return suite.fakeLogs == metrics.LogsProcessed.Value()
	})
	agent.Stop()

	// The context gets canceled when the agent stops. At this point the additional sender is stuck
	// trying to establish a connection. `agent.Stop()` will cancel it and the error telemetry will be updated
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 2*time.Second, func() bool {
		return int64(2) == metrics.DestinationErrors.Value()
	})

	assert.Equal(suite.T(), suite.fakeLogs, metrics.LogsDecoded.Value())
	assert.Equal(suite.T(), suite.fakeLogs, metrics.LogsProcessed.Value())
	assert.Equal(suite.T(), int64(0), metrics.LogsSent.Value())
	assert.Equal(suite.T(), "2", metrics.DestinationLogsDropped.Get("fake:").String())
	assert.True(suite.T(), metrics.DestinationErrors.Value() > 0)
}

func TestAgentTestSuite(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}
