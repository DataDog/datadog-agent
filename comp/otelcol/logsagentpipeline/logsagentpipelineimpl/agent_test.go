// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package logsagentpipelineimpl

import (
	"context"
	"testing"
	"time"

	"go.uber.org/fx"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/client/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type AgentTestSuite struct {
	suite.Suite
	fakeLogs int64

	source          *sources.LogSource
	configOverrides map[string]interface{}
}

type testDeps struct {
	fx.In

	Config configComponent.Component
	Log    log.Component
}

func (suite *AgentTestSuite) SetupTest() {
	suite.configOverrides = map[string]interface{}{}
	suite.fakeLogs = 2 // Two lines.
	logConfig := config.LogsConfig{}
	suite.source = sources.NewLogSource("", &logConfig)
	// Shorter grace period for tests.
	suite.configOverrides["logs_config.stop_grace_period"] = 1
}

func (suite *AgentTestSuite) TearDownTest() {
	// Resets the metrics we check.
	metrics.LogsDecoded.Set(0)
	metrics.LogsProcessed.Set(0)
	metrics.LogsSent.Set(0)
	metrics.DestinationErrors.Set(0)
	metrics.DestinationLogsDropped.Init()
}

func createAgent(suite *AgentTestSuite, endpoints *config.Endpoints) *Agent {
	deps := fxutil.Test[testDeps](suite.T(), fx.Options(
		configComponent.MockModule(),
		logimpl.MockModule(),
		fx.Replace(configComponent.MockParams{Overrides: suite.configOverrides}),
	))

	agent := &Agent{
		log:       deps.Log,
		config:    deps.Config,
		endpoints: endpoints,
	}

	agent.setupAgent()

	return agent
}

func (suite *AgentTestSuite) sendTestMessages(agent *Agent) {
	testChannel := agent.GetPipelineProvider().NextPipelineChan()
	testChannel <- message.NewMessage([]byte("test log1"), message.NewOrigin(suite.source), "", 0)
	testChannel <- message.NewMessage([]byte("test log2"), message.NewOrigin(suite.source), "", 0)
}

func (suite *AgentTestSuite) testAgent(endpoints *config.Endpoints) {
	agent := createAgent(suite, endpoints)

	zero := int64(0)
	assert.Equal(suite.T(), zero, metrics.LogsDecoded.Value())
	assert.Equal(suite.T(), zero, metrics.LogsProcessed.Value())
	assert.Equal(suite.T(), zero, metrics.LogsSent.Value())
	assert.Equal(suite.T(), zero, metrics.DestinationErrors.Value())
	assert.Equal(suite.T(), "{}", metrics.DestinationLogsDropped.String())

	agent.startPipeline()
	suite.sendTestMessages(agent)

	// Give the agent at most 4 second to send the logs. (seems to be slow on Windows/AppVeyor)
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 4*time.Second, func() bool {
		return suite.fakeLogs == metrics.LogsSent.Value()
	})
	agent.Stop(context.TODO())

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
	server := http.NewTestServer(200, pkgconfigsetup.Datadog())
	defer server.Stop()
	endpoints := config.NewEndpoints(server.Endpoint, nil, false, true)

	suite.testAgent(endpoints)
}

func (suite *AgentTestSuite) TestAgentStopsWithWrongBackendTcp() {
	endpoint := config.NewEndpoint("", "fake:", 0, false)
	endpoints := config.NewEndpoints(endpoint, []config.Endpoint{}, true, false)

	agent := createAgent(suite, endpoints)

	agent.startPipeline()
	suite.sendTestMessages(agent)
	// Give the agent at most one second to process the logs.
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 2*time.Second, func() bool {
		return suite.fakeLogs == metrics.LogsProcessed.Value()
	})
	agent.Stop(context.TODO())

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

func (suite *AgentTestSuite) TestGetPipelineProvider() {
	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()

	endpoint := tcp.AddrToEndPoint(l.Addr())
	endpoints := config.NewEndpoints(endpoint, nil, true, false)

	agent := createAgent(suite, endpoints)
	agent.Start(context.Background())

	assert.NotNil(suite.T(), agent.GetPipelineProvider())
}

func TestAgentTestSuite(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}

func TestBuildEndpoints(t *testing.T) {
	config := fxutil.Test[configComponent.Component](t, fx.Options(
		configComponent.MockModule(),
	))

	endpoints, err := buildEndpoints(config, nil)
	assert.Nil(t, err)
	assert.Equal(t, "agent-intake.logs.datadoghq.com", endpoints.Main.Host)
}
