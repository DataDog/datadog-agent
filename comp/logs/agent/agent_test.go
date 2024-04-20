// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/atomic"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"

	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/client/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	logsStatus "github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

type AgentTestSuite struct {
	suite.Suite
	testDir     string
	testLogFile string
	fakeLogs    int64

	source          *sources.LogSource
	configOverrides map[string]interface{}
}

type testDeps struct {
	fx.In

	Config         configComponent.Component
	Log            log.Component
	InventoryAgent inventoryagent.Component
}

func (suite *AgentTestSuite) SetupTest() {
	suite.configOverrides = map[string]interface{}{}

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

	suite.configOverrides["logs_config.run_path"] = suite.testDir
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

func createAgent(suite *AgentTestSuite, endpoints *config.Endpoints) (*agent, *sources.LogSources, *service.Services) {
	// setup the sources and the services
	sources := sources.NewLogSources()
	services := service.NewServices()

	suite.configOverrides["logs_enabled"] = true

	deps := fxutil.Test[testDeps](suite.T(), fx.Options(
		core.MockBundle(),
		fx.Replace(configComponent.MockParams{Overrides: suite.configOverrides}),
		inventoryagentimpl.MockModule(),
	))

	agent := &agent{
		log:            deps.Log,
		config:         deps.Config,
		inventoryAgent: deps.InventoryAgent,
		started:        atomic.NewBool(false),

		sources:   sources,
		services:  services,
		tracker:   tailers.NewTailerTracker(),
		endpoints: endpoints,
	}

	agent.setupAgent()

	return agent, sources, services
}

func (suite *AgentTestSuite) testAgent(endpoints *config.Endpoints) {
	coreConfig.SetFeatures(suite.T(), coreConfig.Docker, coreConfig.Kubernetes)

	agent, sources, _ := createAgent(suite, endpoints)

	zero := int64(0)
	assert.Equal(suite.T(), zero, metrics.LogsDecoded.Value())
	assert.Equal(suite.T(), zero, metrics.LogsProcessed.Value())
	assert.Equal(suite.T(), zero, metrics.LogsSent.Value())
	assert.Equal(suite.T(), zero, metrics.DestinationErrors.Value())
	assert.Equal(suite.T(), "{}", metrics.DestinationLogsDropped.String())

	agent.startPipeline()
	sources.AddSource(suite.source)
	// Give the agent at most 4 second to send the logs. (seems to be slow on Windows/AppVeyor)
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 4*time.Second, func() bool {
		return suite.fakeLogs == metrics.LogsSent.Value()
	})
	agent.stop(context.TODO())

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
	server := http.NewTestServer(200, coreConfig.Datadog)
	defer server.Stop()
	endpoints := config.NewEndpoints(server.Endpoint, nil, false, true)

	suite.testAgent(endpoints)
}

func (suite *AgentTestSuite) TestAgentStopsWithWrongBackendTcp() {
	endpoint := config.NewEndpoint("", "fake:", 0, false)
	endpoints := config.NewEndpoints(endpoint, []config.Endpoint{}, true, false)

	coreConfig.SetFeatures(suite.T(), coreConfig.Docker, coreConfig.Kubernetes)

	agent, sources, _ := createAgent(suite, endpoints)

	agent.startPipeline()
	sources.AddSource(suite.source)
	// Give the agent at most one second to process the logs.
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 2*time.Second, func() bool {
		return suite.fakeLogs == metrics.LogsProcessed.Value()
	})
	agent.stop(context.TODO())

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

	agent, _, _ := createAgent(suite, endpoints)
	agent.Start()

	assert.NotNil(suite.T(), agent.GetPipelineProvider())
}

func (suite *AgentTestSuite) TestStatusProvider() {
	tests := []struct {
		name     string
		enabled  bool
		expected interface{}
	}{
		{
			"logs enabled",
			true,
			NewStatusProvider(),
		},
		{
			"logs disabled",
			false,
			NewStatusProvider(),
		},
	}

	for _, test := range tests {
		suite.T().Run(test.name, func(*testing.T) {
			suite.configOverrides["logs_enabled"] = test.enabled

			deps := suite.createDeps()

			provides := newLogsAgent(deps)

			assert.IsType(suite.T(), test.expected, provides.StatusProvider.Provider)
		})
	}
}

func (suite *AgentTestSuite) TestStatusOut() {
	originalProvider := logsProvider

	mockResult := logsStatus.Status{
		IsRunning: true,
		Endpoints: []string{"foo", "bar"},
		StatusMetrics: map[string]string{
			"hello": "12",
			"world": "13",
		},
		ProcessFileStats: map[string]uint64{
			"CoreAgentProcessOpenFiles": 27,
			"OSFileLimit":               1048576,
		},
		Integrations: []logsStatus.Integration{},
		Tailers:      []logsStatus.Tailer{},
		Errors:       []string{},
		Warnings:     []string{},
		UseHTTP:      true,
	}

	logsProvider = func(verbose bool) logsStatus.Status {
		return mockResult
	}

	defer func() {
		logsProvider = originalProvider
	}()

	suite.configOverrides["logs_enabled"] = true

	deps := suite.createDeps()

	provides := newLogsAgent(deps)

	headerProvider := provides.StatusProvider.Provider

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerProvider.JSON(false, stats)

			assert.Equal(t, mockResult, stats["logsStats"])
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.Text(false, b)

			assert.NoError(t, err)
			result := `
    foo
    bar
    hello: 12
    world: 13
    CoreAgentProcessOpenFiles: 27
    OSFileLimit: 1048576
`
			// We replace windows line break by linux so the tests pass on every OS
			expectedResult := strings.Replace(result, "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)

			assert.Equal(t, expectedResult, output)
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.HTML(false, b)

			assert.NoError(t, err)

			result := `<div class="stat">
  <span class="stat_title">Logs Agent</span>
  <span class="stat_data">
        foo<br>
        bar<br>
        hello: 12<br>
        world: 13<br></span>
</div>
`
			// We replace windows line break by linux so the tests pass on every OS
			expectedResult := strings.Replace(result, "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)

			assert.Equal(t, expectedResult, output)
		}},
	}

	for _, test := range tests {
		suite.T().Run(test.name, func(t *testing.T) {
			test.assertFunc(suite.T())
		})
	}
}

func (suite *AgentTestSuite) TestFlareProvider() {
	tests := []struct {
		name     string
		enabled  bool
		expected interface{}
	}{
		{
			"logs enabled",
			true,
			flaretypes.NewProvider(flareController.NewFlareController().FillFlare),
		},
		{
			"logs disabled",
			false,
			flaretypes.Provider{},
		},
	}

	for _, test := range tests {
		suite.T().Run(test.name, func(*testing.T) {
			suite.configOverrides["logs_enabled"] = test.enabled

			deps := suite.createDeps()

			provides := newLogsAgent(deps)

			assert.IsType(suite.T(), test.expected, provides.FlareProvider)
			if test.enabled {
				assert.NotNil(suite.T(), provides.FlareProvider.Provider)
			} else {
				assert.Nil(suite.T(), provides.FlareProvider.Provider)
			}
		})
	}
}

func (suite *AgentTestSuite) createDeps() dependencies {
	return fxutil.Test[dependencies](suite.T(), fx.Options(
		core.MockBundle(),
		fx.Replace(configComponent.MockParams{Overrides: suite.configOverrides}),
		inventoryagentimpl.MockModule(),
		workloadmeta.MockModule(),
		fx.Supply(workloadmeta.NewParams()),
	))
}

func TestAgentTestSuite(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}
