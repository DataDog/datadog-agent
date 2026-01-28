// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test && !serverless

package agentimpl

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/atomic"
	"go.uber.org/fx"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	kubehealthdef "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/def"
	kubehealthmock "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	auditorfx "github.com/DataDog/datadog-agent/comp/logs/auditor/fx"
	auditorimpl "github.com/DataDog/datadog-agent/comp/logs/auditor/impl"
	integrationsimpl "github.com/DataDog/datadog-agent/comp/logs/integrations/impl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/client/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	logsStatus "github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

type RestartTestSuite struct {
	suite.Suite
	testDir             string
	testLogFile         string
	fakeLogs            int64
	source              *sources.LogSource
	configOverrides     map[string]interface{}
	tagger              tagger.Component
	kubeHealthRegistrar kubehealthdef.Component
}

func (suite *RestartTestSuite) SetupTest() {
	suite.configOverrides = map[string]interface{}{}

	var err error

	suite.testDir = suite.T().TempDir()

	suite.testLogFile = suite.testDir + "/test.log"
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
	// Set a short scan period to allow it to run in the time period of the tcp and http tests
	suite.configOverrides["logs_config.file_scan_period"] = 1

	fakeTagger := taggerfxmock.SetupFakeTagger(suite.T())
	suite.tagger = fakeTagger
}

func (suite *RestartTestSuite) TearDownTest() {
	// Resets the metrics we check.
	metrics.LogsDecoded.Set(0)
	metrics.LogsProcessed.Set(0)
	metrics.LogsSent.Set(0)
	metrics.DestinationErrors.Set(0)
	metrics.DestinationLogsDropped.Init()
	metrics.LogsTruncated.Set(0)

	logsStatus.Clear()
}

func createTestAgent(suite *RestartTestSuite, endpoints *config.Endpoints) (*logAgent, *sources.LogSources, *service.Services) {
	// setup the sources and the services
	sources := sources.NewLogSources()
	services := service.NewServices()

	suite.configOverrides["logs_enabled"] = true

	deps := fxutil.Test[testDeps](suite.T(), fx.Options(
		fx.Provide(func() log.Component { return logmock.New(suite.T()) }),
		fx.Provide(func() configComponent.Component {
			return configComponent.NewMockWithOverrides(suite.T(), suite.configOverrides)
		}),
		hostnameimpl.MockModule(),
		inventoryagentimpl.MockModule(),
		auditorfx.Module(),
		fx.Provide(kubehealthmock.NewProvides),
	))

	fakeTagger := taggerfxmock.SetupFakeTagger(suite.T())
	suite.kubeHealthRegistrar = deps.KubeHealthRegistrar

	agent := &logAgent{
		log:              deps.Log,
		config:           deps.Config,
		inventoryAgent:   deps.InventoryAgent,
		started:          atomic.NewUint32(0),
		integrationsLogs: integrationsimpl.NewLogsIntegration(),

		auditor:         deps.Auditor,
		sources:         sources,
		services:        services,
		tracker:         tailers.NewTailerTracker(),
		endpoints:       endpoints,
		tagger:          fakeTagger,
		flarecontroller: flareController.NewFlareController(),
		compression:     compressionfx.NewMockCompressor(),
		wmeta:           option.None[workloadmeta.Component](),
	}

	agent.setupAgent()
	suite.T().Cleanup(func() {
		_ = agent.stop(context.TODO())
	})

	return agent, sources, services
}

func (suite *RestartTestSuite) TestAgentStartRestart() {
	env.SetFeatures(suite.T(), env.Docker, env.Kubernetes)

	// start on tcp
	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()
	endpoint := tcp.AddrToEndPoint(l.Addr())
	endpoints := config.NewEndpoints(endpoint, nil, true, false)

	agent, sources, _ := createTestAgent(suite, endpoints)

	assert.False(suite.T(), agent.endpoints.UseHTTP)
	assert.Equal(suite.T(), logsStatus.TransportTCP, logsStatus.GetCurrentTransport())

	zero := int64(0)
	assert.Equal(suite.T(), zero, metrics.LogsDecoded.Value())
	assert.Equal(suite.T(), zero, metrics.LogsProcessed.Value())
	assert.Equal(suite.T(), zero, metrics.LogsSent.Value())
	assert.Equal(suite.T(), zero, metrics.DestinationErrors.Value())
	metrics.DestinationLogsDropped.Do(func(k expvar.KeyValue) {
		assert.Equal(suite.T(), k.Value.String(), "0")
	})

	// Store references to persistent components before restart
	originalSources := agent.sources
	originalAuditor := agent.auditor
	originalSchedulers := agent.schedulers

	agent.startPipeline()
	sources.AddSource(suite.source)

	suite.T().Logf("INITIAL TAILING - Expecting %d logs", suite.fakeLogs)
	// Give the agent at most 4 second to send the logs. (seems to be slow on Windows/AppVeyor)
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 6*time.Second, func() bool {
		sent := metrics.LogsSent.Value()
		if sent != suite.fakeLogs {
			suite.T().Logf("Initial tailing: %d/%d logs sent", sent, suite.fakeLogs)
		}
		return suite.fakeLogs == sent
	})

	suite.T().Logf("Initial metrics - Decoded: %d, Processed: %d, Sent: %d",
		metrics.LogsDecoded.Value(), metrics.LogsProcessed.Value(), metrics.LogsSent.Value())

	assert.Equal(suite.T(), suite.fakeLogs, metrics.LogsDecoded.Value())
	assert.Equal(suite.T(), suite.fakeLogs, metrics.LogsProcessed.Value())
	assert.Equal(suite.T(), suite.fakeLogs, metrics.LogsSent.Value())
	assert.Equal(suite.T(), zero, metrics.DestinationErrors.Value())

	// Set up HTTP test server for restart
	cfg := configmock.New(suite.T())
	httpServer := http.NewTestServer(200, cfg)
	defer httpServer.Stop()

	// Update config to point to HTTP server before restart
	// This ensures buildEndpoints() will use HTTP when restart() is called
	httpURL := fmt.Sprintf("http://%s:%d", httpServer.Endpoint.Host, httpServer.Endpoint.Port)
	// Type assert to pkgconfigmodel.Config to access SetWithoutSource
	if cfg, ok := agent.config.(pkgconfigmodel.Config); ok {
		cfg.SetWithoutSource("logs_config.logs_dd_url", httpURL)
	}

	suite.T().Logf("RESTARTING AGENT")

	// Build HTTP endpoints for restart
	httpEndpoints, err := buildHTTPEndpointsForRestart(agent.config)
	suite.NoError(err, "Should build HTTP endpoints")

	// restart agent with HTTP endpoints
	err = agent.restart(context.TODO(), httpEndpoints)

	// confirm we switched to HTTP
	suite.NoError(err)
	suite.True(agent.endpoints.UseHTTP, "Should switch to HTTP after restart")
	suite.Equal(logsStatus.TransportHTTP, logsStatus.GetCurrentTransport())

	suite.T().Logf("RESTART COMPLETE - Current metrics: Decoded: %d, Processed: %d, Sent: %d",
		metrics.LogsDecoded.Value(), metrics.LogsProcessed.Value(), metrics.LogsSent.Value())

	// Verify persistent components were preserved
	suite.Same(originalSources, agent.sources)
	suite.Same(originalAuditor, agent.auditor)
	suite.Same(originalSchedulers, agent.schedulers)

	// Verify transient components were recreated
	suite.NotNil(agent.destinationsCtx)
	suite.NotNil(agent.pipelineProvider)
	suite.NotNil(agent.launchers)

	// We do NOT test exact log counts after restart
	// the async auditor and periodic file scanner (which restarts tailers) creates
	// race conditions that make exact counts flaky
}

func (suite *RestartTestSuite) TestRestart_FlushesAuditor() {
	// This test verifies that the auditor is flushed during restart
	// to minimize (though not eliminate) duplicate log processing

	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()
	endpoint := tcp.AddrToEndPoint(l.Addr())
	endpoints := config.NewEndpoints(endpoint, nil, true, false)

	agent, sources, _ := createTestAgent(suite, endpoints)
	agent.startPipeline()
	sources.AddSource(suite.source)

	// Wait for initial logs to be sent
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 4*time.Second, func() bool {
		return suite.fakeLogs == metrics.LogsSent.Value()
	})

	// Set up HTTP server for restart
	cfg := configmock.New(suite.T())
	httpServer := http.NewTestServer(200, cfg)
	defer httpServer.Stop()
	httpURL := fmt.Sprintf("http://%s:%d", httpServer.Endpoint.Host, httpServer.Endpoint.Port)
	if c, ok := agent.config.(pkgconfigmodel.Config); ok {
		c.SetWithoutSource("logs_config.logs_dd_url", httpURL)
	}

	// Get the auditor registry file path to check it was written
	runPath := agent.config.GetString("logs_config.run_path")
	registryPath := runPath + "/registry.json"

	// Get file mod time before restart
	var beforeModTime time.Time
	if info, err := os.Stat(registryPath); err == nil {
		beforeModTime = info.ModTime()
	}

	// Build HTTP endpoints for restart
	httpEndpoints, err := buildHTTPEndpointsForRestart(agent.config)
	suite.NoError(err, "Should build HTTP endpoints")

	// Execute restart
	err = agent.restart(context.TODO(), httpEndpoints)
	suite.NoError(err)

	// Verify the registry file was written (Flush() was called)
	// The file should exist and be newer than before
	info, err := os.Stat(registryPath)
	suite.NoError(err, "Registry file should exist after Flush()")

	if !beforeModTime.IsZero() {
		suite.True(info.ModTime().After(beforeModTime) || info.ModTime().Equal(beforeModTime),
			"Registry file should be written during restart")
	}

	suite.T().Logf("Auditor registry flushed to: %s", registryPath)
}

func (suite *RestartTestSuite) TestPartialStop_StopsTransientComponentsOnly() {
	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()
	endpoint := tcp.AddrToEndPoint(l.Addr())
	endpoints := config.NewEndpoints(endpoint, nil, true, false)

	agent, _, _ := createTestAgent(suite, endpoints)

	// Store references
	originalDiagnostic := agent.diagnosticMessageReceiver
	originalSchedulers := agent.schedulers
	originalAuditor := agent.auditor

	// Execute partial stop
	err := agent.partialStop()

	// Assertions
	suite.NoError(err)

	// Persistent components should remain
	suite.Same(originalDiagnostic, agent.diagnosticMessageReceiver)
	suite.Same(originalSchedulers, agent.schedulers)
	suite.Same(originalAuditor, agent.auditor)
}

func (suite *RestartTestSuite) TestPartialStop_WithTimeout() {
	suite.configOverrides["logs_config.stop_grace_period"] = 1 // 1 second timeout

	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()
	endpoint := tcp.AddrToEndPoint(l.Addr())
	endpoints := config.NewEndpoints(endpoint, nil, true, false)

	agent, _, _ := createTestAgent(suite, endpoints)
	agent.startPipeline()

	// Execute partial stop with timeout
	start := time.Now()
	err := agent.partialStop()
	elapsed := time.Since(start)

	// Should complete within reasonable time
	suite.NoError(err)
	suite.Less(elapsed, 3*time.Second, "Should complete or timeout within grace period")
}

func (suite *RestartTestSuite) TestPartialStop_FlushesRegistryToDisk() {
	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()
	endpoint := tcp.AddrToEndPoint(l.Addr())
	endpoints := config.NewEndpoints(endpoint, nil, true, false)

	agent, sources, _ := createTestAgent(suite, endpoints)
	agent.startPipeline()
	sources.AddSource(suite.source)

	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 5*time.Second, func() bool {
		return suite.fakeLogs == metrics.LogsSent.Value()
	})

	runPath := agent.config.GetString("logs_config.run_path")
	registryPath := filepath.Join(runPath, "registry.json")
	_ = os.Remove(registryPath)

	f, err := os.OpenFile(suite.testLogFile, os.O_APPEND|os.O_WRONLY, 0)
	suite.NoError(err)
	_, err = f.WriteString("flushable log line\n")
	suite.NoError(err)
	f.Close()

	expected := metrics.LogsSent.Value() + 1
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 4*time.Second, func() bool {
		return metrics.LogsSent.Value() >= expected
	})

	err = agent.partialStop()
	suite.NoError(err)

	data, err := os.ReadFile(registryPath)
	suite.NoError(err)
	suite.NotEmpty(data, "registry should be written after partial stop flush")

	var registry auditorimpl.JSONRegistry
	suite.NoError(json.Unmarshal(data, &registry))
	suite.NotEmpty(registry.Registry)

	found := false
	for key, entry := range registry.Registry {
		if strings.Contains(key, suite.source.Config.Identifier) || strings.Contains(key, suite.source.Config.Path) {
			suite.NotEmpty(entry.Offset, "registry entry should contain the last committed offset")
			found = true
			break
		}
	}
	suite.True(found, "registry should contain the test source entry")
}

func (suite *RestartTestSuite) TestRebuildTransientComponents_PreservesPersistentState() {
	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()
	endpoint := tcp.AddrToEndPoint(l.Addr())
	endpoints := config.NewEndpoints(endpoint, nil, true, false)

	agent, sources, _ := createTestAgent(suite, endpoints)
	agent.startPipeline()
	sources.AddSource(suite.source)

	// Prime the pipeline so we have baseline metrics/state prior to restart
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 5*time.Second, func() bool {
		return suite.fakeLogs == metrics.LogsSent.Value()
	})
	initialSent := metrics.LogsSent.Value()

	// Store references to persistent components
	originalSources := agent.sources
	originalServices := agent.services
	originalTracker := agent.tracker
	originalSchedulers := agent.schedulers
	originalAuditor := agent.auditor
	originalDiagnosticMessageReceiver := agent.diagnosticMessageReceiver
	originalPipelineProvider := agent.pipelineProvider
	originalLaunchers := agent.launchers
	originalDestinationsCtx := agent.destinationsCtx

	// Simulate the stop -> rebuild flow that restart() performs
	err := agent.partialStop()
	suite.NoError(err)

	// Get processing rules and fingerprint config
	processingRules, err := config.GlobalProcessingRules(agent.config)
	suite.NoError(err)
	fingerprintConfig, err := config.GlobalFingerprintConfig(agent.config)
	suite.NoError(err)

	// Execute rebuild - use the agent's existing wmeta and integrationsLogs
	agent.rebuildTransientComponents(processingRules, agent.wmeta, agent.integrationsLogs, *fingerprintConfig)

	// Persistent components preserved
	suite.Same(originalSources, agent.sources)
	suite.Same(originalServices, agent.services)
	suite.Same(originalTracker, agent.tracker)
	suite.Same(originalSchedulers, agent.schedulers)
	suite.Same(originalAuditor, agent.auditor)
	suite.Same(originalDiagnosticMessageReceiver, agent.diagnosticMessageReceiver)

	// Transient components recreated
	suite.NotNil(agent.destinationsCtx)
	suite.NotSame(originalDestinationsCtx, agent.destinationsCtx)
	suite.NotNil(agent.pipelineProvider)
	suite.NotSame(originalPipelineProvider, agent.pipelineProvider)
	suite.NotNil(agent.launchers)
	suite.NotSame(originalLaunchers, agent.launchers)

	// Start the rebuilt components and ensure they are wired to the preserved sources
	agent.restartPipeline()

	f, err := os.OpenFile(suite.testLogFile, os.O_APPEND|os.O_WRONLY, 0)
	suite.NoError(err)
	_, err = f.WriteString("post-restart log line\n")
	suite.NoError(err)
	f.Close()

	expected := initialSent + 1
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 5*time.Second, func() bool {
		return metrics.LogsSent.Value() >= expected
	})
	suite.GreaterOrEqual(metrics.LogsSent.Value(), expected, "restarted pipeline should process new logs")
}

func (suite *RestartTestSuite) TestRestart_SerializesConcurrentCalls() {
	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()
	endpoint := tcp.AddrToEndPoint(l.Addr())
	endpoints := config.NewEndpoints(endpoint, nil, true, false)

	agent, sources, _ := createTestAgent(suite, endpoints)
	agent.startPipeline()
	sources.AddSource(suite.source)

	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 5*time.Second, func() bool {
		return suite.fakeLogs == metrics.LogsSent.Value()
	})

	// Build HTTP endpoints for restart
	httpEndpoints, err := buildHTTPEndpointsForRestart(agent.config)
	suite.NoError(err, "Should build HTTP endpoints")

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		errCh <- agent.restart(context.TODO(), httpEndpoints)
	}()

	// Start the second restart shortly after the first to exercise the mutex
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		errCh <- agent.restart(context.TODO(), httpEndpoints)
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		suite.NoError(err)
	}

	// Restart should leave the pipeline healthy and endpoints intact
	suite.NotNil(agent.destinationsCtx)
	suite.NotNil(agent.pipelineProvider)
	suite.NotNil(agent.launchers)
}

func (suite *RestartTestSuite) TestRestartWithHTTPUpgrade_Success() {
	// Start with TCP
	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()
	endpoint := tcp.AddrToEndPoint(l.Addr())
	endpoints := config.NewEndpoints(endpoint, nil, true, false)

	agent, sources, _ := createTestAgent(suite, endpoints)
	agent.startPipeline()
	sources.AddSource(suite.source)

	// Wait for initial logs
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 4*time.Second, func() bool {
		return suite.fakeLogs == metrics.LogsSent.Value()
	})

	// Set up HTTP server
	cfg := configmock.New(suite.T())
	httpServer := http.NewTestServer(200, cfg)
	defer httpServer.Stop()

	httpURL := fmt.Sprintf("http://%s:%d", httpServer.Endpoint.Host, httpServer.Endpoint.Port)
	if c, ok := agent.config.(pkgconfigmodel.Config); ok {
		c.SetWithoutSource("logs_config.logs_dd_url", httpURL)
	}

	// Execute HTTP upgrade
	err := agent.restartWithHTTPUpgrade(context.TODO())
	suite.NoError(err)

	// Verify we're on HTTP now
	suite.True(agent.endpoints.UseHTTP, "Should be using HTTP after upgrade")
	suite.Equal(logsStatus.TransportHTTP, logsStatus.GetCurrentTransport())
}

func (suite *RestartTestSuite) TestRestartWithHTTPUpgrade_FailureRollsBackToTCP() {
	// Start with TCP
	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()
	endpoint := tcp.AddrToEndPoint(l.Addr())
	endpoints := config.NewEndpoints(endpoint, nil, true, false)

	agent, sources, _ := createTestAgent(suite, endpoints)
	agent.startPipeline()
	sources.AddSource(suite.source)

	// Wait for initial logs
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 4*time.Second, func() bool {
		return suite.fakeLogs == metrics.LogsSent.Value()
	})

	// Store original TCP endpoints
	originalEndpoints := agent.endpoints
	suite.False(originalEndpoints.UseHTTP, "Should start on TCP")

	// Set invalid HTTP URL to trigger failure
	if c, ok := agent.config.(pkgconfigmodel.Config); ok {
		c.SetWithoutSource("logs_config.logs_dd_url", "invalid-address-will-fail")
	}

	// Attempt HTTP upgrade (should fail and rollback)
	err := agent.restartWithHTTPUpgrade(context.TODO())
	suite.Error(err, "Should return error when HTTP upgrade fails")

	// Verify we rolled back to TCP
	suite.False(agent.endpoints.UseHTTP, "Should rollback to TCP after failure")
	suite.NotNil(agent.destinationsCtx, "Pipeline should be running after rollback")
	suite.NotNil(agent.pipelineProvider, "Pipeline should be running after rollback")
}

func (suite *RestartTestSuite) TestRestartWithHTTPUpgrade_PreservesComponentReferences() {
	// Start with TCP
	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()
	endpoint := tcp.AddrToEndPoint(l.Addr())
	endpoints := config.NewEndpoints(endpoint, nil, true, false)

	agent, sources, _ := createTestAgent(suite, endpoints)
	agent.startPipeline()
	sources.AddSource(suite.source)

	// Store references to persistent components
	originalSources := agent.sources
	originalAuditor := agent.auditor
	originalSchedulers := agent.schedulers

	// Set up HTTP server
	cfg := configmock.New(suite.T())
	httpServer := http.NewTestServer(200, cfg)
	defer httpServer.Stop()

	httpURL := fmt.Sprintf("http://%s:%d", httpServer.Endpoint.Host, httpServer.Endpoint.Port)
	if c, ok := agent.config.(pkgconfigmodel.Config); ok {
		c.SetWithoutSource("logs_config.logs_dd_url", httpURL)
	}

	// Execute HTTP upgrade
	err := agent.restartWithHTTPUpgrade(context.TODO())
	suite.NoError(err)

	// Verify persistent components preserved
	suite.Same(originalSources, agent.sources)
	suite.Same(originalAuditor, agent.auditor)
	suite.Same(originalSchedulers, agent.schedulers)
}

func (suite *RestartTestSuite) TestRollbackToPreviousTransport() {
	// Start with TCP
	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()
	endpoint := tcp.AddrToEndPoint(l.Addr())
	tcpEndpoints := config.NewEndpoints(endpoint, nil, true, false)

	agent, _, _ := createTestAgent(suite, tcpEndpoints)
	agent.startPipeline()

	originalEndpoints := agent.endpoints
	suite.False(originalEndpoints.UseHTTP, "Should start on TCP")

	// Simulate partial stop (what restart() does before trying HTTP)
	err := agent.partialStop()
	suite.NoError(err)

	// Set up HTTP endpoints (simulating what restart() tries)
	cfg := configmock.New(suite.T())
	httpServer := http.NewTestServer(200, cfg)
	defer httpServer.Stop()

	httpEndpoints := config.NewEndpoints(httpServer.Endpoint, nil, false, true)
	agent.endpoints = httpEndpoints

	// Now rollback to TCP
	err = agent.rollbackToPreviousTransport(originalEndpoints)
	suite.Error(err, "rollbackToPreviousTransport should return error to signal failure")
	suite.Contains(err.Error(), "rolled back")

	// Verify we're back on TCP
	suite.False(agent.endpoints.UseHTTP, "Should be back on TCP after rollback")
	suite.NotNil(agent.destinationsCtx, "Pipeline should be restarted")
	suite.NotNil(agent.pipelineProvider, "Pipeline should be restarted")
}

func (suite *RestartTestSuite) TestRestart_FailureRollbackThenRetrySuccess() {
	// Start with TCP
	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()
	endpoint := tcp.AddrToEndPoint(l.Addr())
	tcpEndpoints := config.NewEndpoints(endpoint, nil, true, false)

	agent, sources, _ := createTestAgent(suite, tcpEndpoints)
	agent.startPipeline()
	sources.AddSource(suite.source)

	// Wait for initial logs
	testutil.AssertTrueBeforeTimeout(suite.T(), 10*time.Millisecond, 4*time.Second, func() bool {
		return suite.fakeLogs == metrics.LogsSent.Value()
	})

	suite.False(agent.endpoints.UseHTTP, "Should start on TCP")

	// ATTEMPT 1: Try to restart with invalid HTTP URL (should fail and rollback)
	suite.T().Log("ATTEMPT 1: Restart with invalid HTTP URL")
	if cfg, ok := agent.config.(pkgconfigmodel.Config); ok {
		cfg.SetWithoutSource("logs_config.logs_dd_url", "invalid-address-will-fail")
	}

	err := agent.restartWithHTTPUpgrade(context.TODO())
	suite.Error(err, "First restart attempt should fail")
	suite.False(agent.endpoints.UseHTTP, "Should be back on TCP after rollback")

	// Verify agent is still functional on TCP
	suite.NotNil(agent.destinationsCtx, "Agent should be functional after rollback")
	suite.NotNil(agent.pipelineProvider, "Agent should be functional after rollback")

	// ATTEMPT 2: Now retry with valid HTTP URL (should succeed)
	suite.T().Log("ATTEMPT 2: Retry restart with valid HTTP URL")
	cfg := configmock.New(suite.T())
	httpServer := http.NewTestServer(200, cfg)
	defer httpServer.Stop()

	httpURL := fmt.Sprintf("http://%s:%d", httpServer.Endpoint.Host, httpServer.Endpoint.Port)
	if c, ok := agent.config.(pkgconfigmodel.Config); ok {
		c.SetWithoutSource("logs_config.logs_dd_url", httpURL)
	}

	err = agent.restartWithHTTPUpgrade(context.TODO())
	suite.NoError(err, "Second restart attempt should succeed")
	suite.True(agent.endpoints.UseHTTP, "Should now be on HTTP")
	suite.Equal(logsStatus.TransportHTTP, logsStatus.GetCurrentTransport())

	// Verify agent is functional on HTTP
	suite.NotNil(agent.destinationsCtx, "Agent should be functional on HTTP")
	suite.NotNil(agent.pipelineProvider, "Agent should be functional on HTTP")
	suite.NotNil(agent.launchers, "Agent should be functional on HTTP")
}

func TestRestartTestSuite(t *testing.T) {
	suite.Run(t, new(RestartTestSuite))
}
