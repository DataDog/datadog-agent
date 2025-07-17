// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/benbjohnson/clock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	model "github.com/DataDog/agent-payload/v5/process"
	mockStatsd "github.com/DataDog/datadog-go/v5/statsd/mocks"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/process/forwarders"
	"github.com/DataDog/datadog-agent/comp/process/forwarders/forwardersimpl"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestNewCollectorQueueSize(t *testing.T) {
	tests := []struct {
		name              string
		configOverrides   map[string]interface{}
		queueSize         int
		expectedQueueSize int
	}{
		{
			name:              "default queue size",
			configOverrides:   nil,
			queueSize:         42,
			expectedQueueSize: pkgconfigsetup.DefaultProcessQueueSize,
		},
		{
			name: "valid queue size override",
			configOverrides: map[string]interface{}{
				"process_config.queue_size": 42,
			},
			queueSize:         42,
			expectedQueueSize: 42,
		},
		{
			name: "invalid negative queue size override",
			configOverrides: map[string]interface{}{
				"process_config.queue_size": -10,
			},
			queueSize:         -10,
			expectedQueueSize: pkgconfigsetup.DefaultProcessQueueSize,
		},
		{
			name: "invalid 0 queue size override",
			configOverrides: map[string]interface{}{
				"process_config.queue_size": 0,
			},
			queueSize:         0,
			expectedQueueSize: pkgconfigsetup.DefaultProcessQueueSize,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := getSubmitterDeps(t, tc.configOverrides, nil)
			c, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedQueueSize, c.processResults.MaxSize())
		})
	}
}

func TestNewCollectorRTQueueSize(t *testing.T) {
	tests := []struct {
		name              string
		configOverrides   map[string]interface{}
		queueSize         int
		expectedQueueSize int
	}{
		{
			name:              "default queue size",
			configOverrides:   nil,
			queueSize:         2,
			expectedQueueSize: pkgconfigsetup.DefaultProcessRTQueueSize,
		},
		{
			name: "valid queue size override",
			configOverrides: map[string]interface{}{
				"process_config.rt_queue_size": 2,
			},
			queueSize:         2,
			expectedQueueSize: 2,
		},
		{
			name: "invalid negative size override",
			configOverrides: map[string]interface{}{
				"process_config.rt_queue_size": -2,
			},
			queueSize:         -2,
			expectedQueueSize: pkgconfigsetup.DefaultProcessRTQueueSize,
		},
		{
			name: "invalid 0 queue size override",
			configOverrides: map[string]interface{}{
				"process_config.rt_queue_size": 0,
			},
			queueSize:         0,
			expectedQueueSize: pkgconfigsetup.DefaultProcessRTQueueSize,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := getSubmitterDeps(t, tc.configOverrides, nil)
			c, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedQueueSize, c.rtProcessResults.MaxSize())
		})
	}
}

func TestNewCollectorProcessQueueBytes(t *testing.T) {
	tests := []struct {
		name              string
		configOverrides   map[string]interface{}
		queueBytes        int
		expectedQueueSize int
	}{
		{
			name:              "default queue size",
			configOverrides:   nil,
			queueBytes:        42000,
			expectedQueueSize: pkgconfigsetup.DefaultProcessQueueBytes,
		},
		{
			name: "valid queue size override",
			configOverrides: map[string]interface{}{
				"process_config.process_queue_bytes": 42000,
			},
			queueBytes:        42000,
			expectedQueueSize: 42000,
		},
		{
			name: "invalid negative queue size override",
			configOverrides: map[string]interface{}{
				"process_config.process_queue_bytes": -2,
			},
			queueBytes:        -2,
			expectedQueueSize: pkgconfigsetup.DefaultProcessQueueBytes,
		},
		{
			name: "invalid 0 queue size override",
			configOverrides: map[string]interface{}{
				"process_config.process_queue_bytes": 0,
			},
			queueBytes:        0,
			expectedQueueSize: pkgconfigsetup.DefaultProcessQueueBytes,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := getSubmitterDeps(t, tc.configOverrides, nil)
			s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
			assert.NoError(t, err)
			assert.Equal(t, int64(tc.expectedQueueSize), s.processResults.MaxWeight())
			assert.Equal(t, int64(tc.expectedQueueSize), s.rtProcessResults.MaxWeight())
			assert.Equal(t, tc.expectedQueueSize, s.forwarderRetryMaxQueueBytes)
		})
	}
}

func TestCollectorMessagesToCheckResult(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)
	flavor.SetFlavor(flavor.ProcessAgent)

	configOverrides := map[string]interface{}{
		"process_config.process_collection.enabled": "false",
	}
	sysprobeconfigOverrides := map[string]interface{}{
		"discovery.enabled": "false",
	}
	deps := getSubmitterDeps(t, configOverrides, sysprobeconfigOverrides)
	submitter, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
	assert.NoError(t, err)

	agentVersion, _ := version.Agent()
	now := time.Now()

	requestID := submitter.getRequestID(now, 0)

	tests := []struct {
		name          string
		message       model.MessageBody
		expectHeaders map[string]string
	}{
		{
			name: "process",
			message: &model.CollectorProc{
				Containers: []*model.Container{
					{}, {}, {},
				},
			},
			expectHeaders: map[string]string{
				headers.TimestampHeader:         strconv.Itoa(int(now.Unix())),
				headers.HostHeader:              testHostName,
				headers.ProcessVersionHeader:    agentVersion.GetNumber(),
				headers.ContainerCountHeader:    "3",
				headers.ContentTypeHeader:       headers.ProtobufContentType,
				headers.AgentStartTime:          strconv.Itoa(int(submitter.agentStartTime)),
				headers.PayloadSource:           "process_agent",
				headers.RequestIDHeader:         requestID,
				headers.ProcessesEnabled:        "false",
				headers.ServiceDiscoveryEnabled: "false",
			},
		},
		{
			name: "rt_process",
			message: &model.CollectorRealTime{
				ContainerStats: []*model.ContainerStat{
					{}, {}, {},
				},
			},
			expectHeaders: map[string]string{
				headers.TimestampHeader:         strconv.Itoa(int(now.Unix())),
				headers.HostHeader:              testHostName,
				headers.ProcessVersionHeader:    agentVersion.GetNumber(),
				headers.ContainerCountHeader:    "3",
				headers.ContentTypeHeader:       headers.ProtobufContentType,
				headers.AgentStartTime:          strconv.Itoa(int(submitter.agentStartTime)),
				headers.PayloadSource:           "process_agent",
				headers.ProcessesEnabled:        "false",
				headers.ServiceDiscoveryEnabled: "false",
			},
		},
		{
			name: "container",
			message: &model.CollectorContainer{
				Containers: []*model.Container{
					{}, {},
				},
			},
			expectHeaders: map[string]string{
				headers.TimestampHeader:         strconv.Itoa(int(now.Unix())),
				headers.HostHeader:              testHostName,
				headers.ProcessVersionHeader:    agentVersion.GetNumber(),
				headers.ContainerCountHeader:    "2",
				headers.ContentTypeHeader:       headers.ProtobufContentType,
				headers.AgentStartTime:          strconv.Itoa(int(submitter.agentStartTime)),
				headers.PayloadSource:           "process_agent",
				headers.ProcessesEnabled:        "false",
				headers.ServiceDiscoveryEnabled: "false",
			},
		},
		{
			name: "rt_container",
			message: &model.CollectorContainerRealTime{
				Stats: []*model.ContainerStat{
					{}, {}, {}, {}, {},
				},
			},
			expectHeaders: map[string]string{
				headers.TimestampHeader:         strconv.Itoa(int(now.Unix())),
				headers.HostHeader:              testHostName,
				headers.ProcessVersionHeader:    agentVersion.GetNumber(),
				headers.ContainerCountHeader:    "5",
				headers.ContentTypeHeader:       headers.ProtobufContentType,
				headers.AgentStartTime:          strconv.Itoa(int(submitter.agentStartTime)),
				headers.PayloadSource:           "process_agent",
				headers.ProcessesEnabled:        "false",
				headers.ServiceDiscoveryEnabled: "false",
			},
		},
		{
			name:    "process_discovery",
			message: &model.CollectorProcDiscovery{},
			expectHeaders: map[string]string{
				headers.TimestampHeader:         strconv.Itoa(int(now.Unix())),
				headers.HostHeader:              testHostName,
				headers.ProcessVersionHeader:    agentVersion.GetNumber(),
				headers.ContainerCountHeader:    "0",
				headers.ContentTypeHeader:       headers.ProtobufContentType,
				headers.AgentStartTime:          strconv.Itoa(int(submitter.agentStartTime)),
				headers.PayloadSource:           "process_agent",
				headers.ProcessesEnabled:        "false",
				headers.ServiceDiscoveryEnabled: "false",
			},
		},
		{
			name:    "process_events",
			message: &model.CollectorProcEvent{},
			expectHeaders: map[string]string{
				headers.TimestampHeader:         strconv.Itoa(int(now.Unix())),
				headers.HostHeader:              testHostName,
				headers.ProcessVersionHeader:    agentVersion.GetNumber(),
				headers.ContainerCountHeader:    "0",
				headers.ContentTypeHeader:       headers.ProtobufContentType,
				headers.EVPOriginHeader:         "process-agent",
				headers.EVPOriginVersionHeader:  version.AgentVersion,
				headers.AgentStartTime:          strconv.Itoa(int(submitter.agentStartTime)),
				headers.PayloadSource:           "process_agent",
				headers.ProcessesEnabled:        "false",
				headers.ServiceDiscoveryEnabled: "false",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			messages := []model.MessageBody{
				test.message,
			}
			result := submitter.messagesToCheckResult(now, test.name, messages)
			assert.Equal(t, test.name, result.name)
			assert.Len(t, result.payloads, 1)
			payload := result.payloads[0]
			assert.Len(t, payload.headers, len(test.expectHeaders))
			for k, v := range test.expectHeaders {
				assert.Equal(t, v, payload.headers.Get(k))
			}
		})
	}
}

func Test_getRequestID(t *testing.T) {
	deps := getSubmitterDeps(t, nil, nil)
	s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
	assert.NoError(t, err)

	fixedDate1 := time.Date(2022, 9, 1, 0, 0, 1, 0, time.Local)
	id1 := s.getRequestID(fixedDate1, 1)
	id2 := s.getRequestID(fixedDate1, 1)
	// The calculation should be deterministic, so making sure the parameters generates the same id.
	assert.Equal(t, id1, id2)
	fixedDate2 := time.Date(2022, 9, 1, 0, 0, 2, 0, time.Local)
	id3 := s.getRequestID(fixedDate2, 1)

	// The request id is based on time, so if the difference it only the time, then the new ID should be greater.
	id1Num, _ := strconv.ParseUint(id1, 10, 64)
	id3Num, _ := strconv.ParseUint(id3, 10, 64)
	assert.Greater(t, id3Num, id1Num)

	// Increasing the chunk index should increase the id.
	id4 := s.getRequestID(fixedDate2, 3)
	id4Num, _ := strconv.ParseUint(id4, 10, 64)
	assert.Equal(t, id3Num+2, id4Num)

	// Changing the host -> changing the hash.
	s.hostname = "host2"
	s.requestIDCachedHash = nil
	id5 := s.getRequestID(fixedDate1, 1)
	assert.NotEqual(t, id1, id5)
}

func TestSubmitterHeartbeatProcess(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)
	flavor.SetFlavor(flavor.ProcessAgent)

	ctrl := gomock.NewController(t)
	statsdClient := mockStatsd.NewMockClientInterface(ctrl)
	statsdClient.EXPECT().Gauge("datadog.process.agent", float64(1), gomock.Any(), float64(1)).MinTimes(1)

	deps := getSubmitterDeps(t, nil, nil)
	s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, statsdClient, testHostName, deps.SysProbeConfig)
	assert.NoError(t, err)
	mockedClock := clock.NewMock()
	s.clock = mockedClock
	s.Start()
	mockedClock.Add(15 * time.Second)
	s.Stop()
}

func TestSubmitterHeartbeatCore(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)
	flavor.SetFlavor(flavor.DefaultAgent)

	ctrl := gomock.NewController(t)
	statsdClient := mockStatsd.NewMockClientInterface(ctrl)
	statsdClient.EXPECT().Gauge("datadog.process.agent", float64(1), gomock.Any(), float64(1)).Times(0)

	deps := getSubmitterDeps(t, nil, nil)
	s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, statsdClient, testHostName, deps.SysProbeConfig)
	assert.NoError(t, err)
	mockedClock := clock.NewMock()
	s.clock = mockedClock
	s.Start()
	mockedClock.Add(15 * time.Second)
	s.Stop()
}

func TestSubmitterFeatureHeaders(t *testing.T) {
	tests := []struct {
		name                       string
		configOverrides            map[string]interface{}
		sysprobeconfigOverrides    map[string]interface{}
		expProcessesEnabled        string
		expServiceDiscoveryEnabled string
	}{
		{
			name: "just processes enabled",
			configOverrides: map[string]interface{}{
				"process_config.process_collection.enabled": "true",
			},
			sysprobeconfigOverrides: map[string]interface{}{
				"discovery.enabled": "false",
			},
			expProcessesEnabled:        "true",
			expServiceDiscoveryEnabled: "false",
		},
		{
			name: "just service discovery enabled",
			configOverrides: map[string]interface{}{
				"process_config.process_collection.enabled": "false",
			},
			sysprobeconfigOverrides: map[string]interface{}{
				"discovery.enabled": "true",
			},
			expProcessesEnabled:        "false",
			expServiceDiscoveryEnabled: "true",
		},
		{
			name: "both enabled",
			configOverrides: map[string]interface{}{
				"process_config.process_collection.enabled": "true",
			},
			sysprobeconfigOverrides: map[string]interface{}{
				"discovery.enabled": "true",
			},
			expProcessesEnabled:        "true",
			expServiceDiscoveryEnabled: "true",
		},
		{
			name: "both disabled",
			configOverrides: map[string]interface{}{
				"process_config.process_collection.enabled": "false",
			},
			sysprobeconfigOverrides: map[string]interface{}{
				"discovery.enabled": "false",
			},
			expProcessesEnabled:        "false",
			expServiceDiscoveryEnabled: "false",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := getSubmitterDeps(t, tc.configOverrides, tc.sysprobeconfigOverrides)
			s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
			assert.NoError(t, err)
			assert.Equal(t, tc.expProcessesEnabled, s.processesEnabled)
			assert.Equal(t, tc.expServiceDiscoveryEnabled, s.serviceDiscoveryEnabled)
		})
	}
}

type submitterDeps struct {
	fx.In
	Config         config.Component
	SysProbeConfig sysprobeconfig.Component
	Log            log.Component
	Forwarders     forwarders.Component
	Statsd         statsd.ClientInterface
}

func getSubmitterDeps(t *testing.T, configOverrides map[string]interface{}, sysprobeconfigOverrides map[string]interface{}) submitterDeps {
	return fxutil.Test[submitterDeps](t, fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: configOverrides}),
		sysprobeconfigimpl.MockModule(),
		fx.Replace(sysprobeconfigimpl.MockParams{Overrides: sysprobeconfigOverrides}),
		forwardersimpl.MockModule(),
		fx.Provide(func() log.Component {
			return logmock.New(t)
		}),
		fx.Provide(func() statsd.ClientInterface {
			return &statsd.NoOpClient{}
		}),
	))
}
