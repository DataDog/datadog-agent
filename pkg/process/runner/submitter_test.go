// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package runner

import (
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/golang/mock/gomock" //nolint:depguard // required by datadog-go/v5 statsd mocks compiled against golang/mock
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	model "github.com/DataDog/agent-payload/v5/process"
	mockStatsd "github.com/DataDog/datadog-go/v5/statsd/mocks"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsfxnoop "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	connectionsforwarderfx "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/fx"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	forwarders "github.com/DataDog/datadog-agent/comp/process/forwarders/def"
	forwardersimpl "github.com/DataDog/datadog-agent/comp/process/forwarders/fx"
	"github.com/DataDog/datadog-agent/comp/process/types"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
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
			assert.Equal(t, tc.expectedQueueSize, c.resultsQueue[checks.ProcessCheckName].MaxSize())
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
			assert.Equal(t, tc.expectedQueueSize, c.resultsQueue[checks.RTProcessCheckName].MaxSize())
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
			assert.Equal(t, int64(tc.expectedQueueSize), s.resultsQueue[checks.ProcessCheckName].MaxWeight())
			assert.Equal(t, int64(tc.expectedQueueSize), s.resultsQueue[checks.RTProcessCheckName].MaxWeight())
			assert.Equal(t, tc.expectedQueueSize, s.forwarderRetryMaxQueueBytes)
		})
	}
}

// TestSubmitterDoesNotBlockHealthyDomainOnSlowDomain is a regression test for CXP-3920.
// A healthy endpoint's realtime feedback and the next process payload must not
// wait for another endpoint's response channel to close.
func TestSubmitterDoesNotBlockHealthyDomainOnSlowDomain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)

		var calls atomic.Int32
		release := make(chan struct{})
		releaseOnce := sync.OnceFunc(func() { close(release) })
		t.Cleanup(func() {
			releaseOnce()
			s.Stop()
		})
		healthyBody := encodeCollectorResponse(t, 1, 2)

		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			calls.Add(1)
			responses := make(chan forwarder.Response, 1)
			responses <- forwarder.Response{
				Domain:     "https://healthy.example.com",
				StatusCode: http.StatusAccepted,
				Body:       healthyBody,
			}
			go func() {
				<-release // The additional endpoint has not replied yet.
				close(responses)
			}()
			return responses, nil
		}
		require.NoError(t, s.Start())

		submitProcessPayload := func() {
			s.Submit(time.Now(), checks.ProcessCheckName, &types.Payload{
				CheckName: checks.ProcessCheckName,
				Message:   []model.MessageBody{&model.CollectorProc{HostName: testHostName}},
			})
		}
		submitProcessPayload()
		synctest.Wait()
		require.EqualValues(t, 1, calls.Load())
		notifier := s.GetRTNotifier()
		select {
		case <-notifier.Wake:
			response := notifier.Latest()
			require.Len(t, response.Statuses, 1, "healthy response should enable realtime before the slow endpoint is released")
			assert.EqualValues(t, 1, response.Statuses[0].ActiveClients)
		default:
			t.Fatal("healthy endpoint's realtime status waited for the slow endpoint")
		}

		submitProcessPayload()
		synctest.Wait()
		require.EqualValues(t, 2, calls.Load(), "second payload waited for the slow endpoint")
	})
}

func TestSubmitterConnectionsRemainOrderedOnSlowDomain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)

		var calls atomic.Int32
		release := make(chan struct{})
		releaseOnce := sync.OnceFunc(func() { close(release) })
		t.Cleanup(func() {
			releaseOnce()
			s.Stop()
		})
		s.submitFuncs[checks.ConnectionsCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			if calls.Add(1) == 1 {
				responses := make(chan forwarder.Response, 1)
				responses <- forwarder.Response{Domain: "https://healthy.example.com", StatusCode: http.StatusAccepted}
				go func() {
					<-release // The first submission's additional endpoint is still pending.
					close(responses)
				}()
				return responses, nil
			}
			responses := make(chan forwarder.Response)
			close(responses)
			return responses, nil
		}
		require.NoError(t, s.Start())

		submitConnections := func() {
			s.Submit(time.Now(), checks.ConnectionsCheckName, &types.Payload{
				CheckName: checks.ConnectionsCheckName,
				Message:   []model.MessageBody{&model.CollectorConnections{HostName: testHostName}},
			})
		}
		submitConnections()
		synctest.Wait()
		require.EqualValues(t, 1, calls.Load())
		submitConnections()
		synctest.Wait()
		require.EqualValues(t, 1, calls.Load(), "connections must wait for every endpoint of the first payload")
		releaseOnce()
		synctest.Wait()
		require.EqualValues(t, 2, calls.Load(), "second connections payload was not submitted after the first completed")
	})
}

func TestSubmitterIgnoresLateSameDomainResponse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)

		first := make(chan forwarder.Response, 1)
		firstClose := sync.OnceFunc(func() { close(first) })
		t.Cleanup(func() {
			firstClose()
			s.Stop()
		})
		var calls atomic.Int32
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			calls.Add(1)
			return first, nil
		}
		s.submitFuncs[checks.RTProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			calls.Add(1)
			second := make(chan forwarder.Response, 1)
			second <- forwarder.Response{Domain: "https://same.example.com", StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 2, 5)}
			close(second)
			return second, nil
		}
		require.NoError(t, s.Start())
		submit := func() {
			s.Submit(time.Now(), checks.ProcessCheckName, &types.Payload{
				CheckName: checks.ProcessCheckName,
				Message:   []model.MessageBody{&model.CollectorProc{HostName: testHostName}},
			})
		}
		submit()
		synctest.Wait()
		require.EqualValues(t, 1, calls.Load())
		s.Submit(time.Now(), checks.RTProcessCheckName, &types.Payload{
			CheckName: checks.RTProcessCheckName,
			Message:   []model.MessageBody{&model.CollectorRealTime{HostName: testHostName}},
		})
		synctest.Wait()
		require.EqualValues(t, 2, calls.Load())
		notifier := s.GetRTNotifier()
		var newest types.RTResponse
		select {
		case <-notifier.Wake:
			newest = notifier.Latest()
		default:
			t.Fatal("newer response was not published")
		}
		require.Len(t, newest.Statuses, 1)
		assert.EqualValues(t, 2, newest.Statuses[0].ActiveClients)

		first <- forwarder.Response{Domain: "https://same.example.com", StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 0, 2)}
		firstClose()
		synctest.Wait()
		select {
		case <-notifier.Wake:
			t.Fatalf("older response replaced newer status: %+v", notifier.Latest())
		default:
		}
	})
}

func TestSubmitterMergesLatestStatusAcrossDomains(t *testing.T) {
	deps := getSubmitterDeps(t, nil, nil)
	s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
	require.NoError(t, err)
	runner, err := NewRunnerWithChecks(deps.Config, nil, &checks.HostInfo{}, nil, true, nil)
	require.NoError(t, err)
	notifier := s.GetRTNotifier()
	response := func() types.RTResponse {
		t.Helper()
		select {
		case <-notifier.Wake:
			return notifier.Latest()
		default:
			t.Fatal("expected immediate realtime publication")
			return types.RTResponse{}
		}
	}

	s.updateRTStatus(rtDestination{domain: "https://active.example.com"}, 1, &model.CollectorStatus{ActiveClients: 1, Interval: 3})
	first := response()
	require.Len(t, first.Statuses, 1)
	assert.EqualValues(t, 1, first.Statuses[0].ActiveClients)
	runner.UpdateRTStatus(first.Statuses)
	assert.True(t, runner.IsRealTimeEnabled())

	s.updateRTStatus(rtDestination{domain: "https://inactive.example.com"}, 2, &model.CollectorStatus{ActiveClients: 0, Interval: 8})
	second := response()
	assert.Greater(t, second.Sequence, first.Sequence)
	require.Len(t, second.Statuses, 2)
	assert.ElementsMatch(t, []*model.CollectorStatus{{ActiveClients: 1, Interval: 3}, {ActiveClients: 0, Interval: 8}}, second.Statuses)
	runner.UpdateRTStatus(second.Statuses)
	assert.True(t, runner.IsRealTimeEnabled(), "inactive endpoint must not turn off another endpoint's realtime clients")
	assert.Equal(t, 8*time.Second, runner.realTimeInterval)

	// The active endpoint still counts while only the other endpoint replies again.
	s.updateRTStatus(rtDestination{domain: "https://inactive.example.com"}, 3, &model.CollectorStatus{ActiveClients: 0, Interval: 9})
	third := response()
	assert.ElementsMatch(t, []*model.CollectorStatus{{ActiveClients: 1, Interval: 3}, {ActiveClients: 0, Interval: 9}}, third.Statuses)
	runner.UpdateRTStatus(third.Statuses)
	assert.True(t, runner.IsRealTimeEnabled(), "an unanswered endpoint retains its last status")
	assert.Equal(t, 9*time.Second, runner.realTimeInterval)

	s.updateRTStatus(rtDestination{domain: "https://active.example.com"}, 4, &model.CollectorStatus{ActiveClients: 0, Interval: 4})
	last := response()
	assert.Greater(t, last.Sequence, third.Sequence)
	assert.ElementsMatch(t, []*model.CollectorStatus{{ActiveClients: 0, Interval: 4}, {ActiveClients: 0, Interval: 9}}, last.Statuses)
	runner.UpdateRTStatus(last.Statuses)
	assert.False(t, runner.IsRealTimeEnabled(), "realtime turns off only when every endpoint reports no clients")
}

func TestSubmitterKeepsStatusesForDistinctAPIKeys(t *testing.T) {
	deps := getSubmitterDeps(t, nil, nil)
	s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
	require.NoError(t, err)
	notifier := s.GetRTNotifier()

	active := rtDestination{domain: "https://same.example.com", apiKeyIndex: 0}
	inactive := rtDestination{domain: "https://same.example.com", apiKeyIndex: 1}
	s.updateRTStatus(active, 1, &model.CollectorStatus{ActiveClients: 1, Interval: 3})
	<-notifier.Wake
	s.updateRTStatus(inactive, 1, &model.CollectorStatus{ActiveClients: 0, Interval: 5})
	<-notifier.Wake
	assert.ElementsMatch(t, []*model.CollectorStatus{{ActiveClients: 1, Interval: 3}, {ActiveClients: 0, Interval: 5}}, notifier.Latest().Statuses)

	// Older feedback for one key must not replace it, but must not block the other key.
	s.updateRTStatus(active, 2, &model.CollectorStatus{ActiveClients: 2})
	<-notifier.Wake
	s.updateRTStatus(active, 1, &model.CollectorStatus{ActiveClients: 0})
	assert.ElementsMatch(t, []*model.CollectorStatus{{ActiveClients: 2}, {ActiveClients: 0, Interval: 5}}, notifier.Latest().Statuses)
}

func TestSubmitterPublishesExpirationWithoutFurtherResponses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		require.NoError(t, s.Start())
		defer s.Stop()

		notifier := s.GetRTNotifier()
		s.updateRTStatus(rtDestination{domain: "https://unreachable.example.com"}, 1, &model.CollectorStatus{ActiveClients: 1})
		<-notifier.Wake
		time.Sleep(rtStatusTTL + 10*time.Second)
		synctest.Wait()
		select {
		case <-notifier.Wake:
			assert.Empty(t, notifier.Latest().Statuses)
		default:
			t.Fatal("stale feedback was not removed without another response")
		}
	})
}

func TestSubmitterExpiresStaleStatusAndRejectsOlderFeedback(t *testing.T) {
	deps := getSubmitterDeps(t, nil, nil)
	s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
	require.NoError(t, err)
	notifier := s.GetRTNotifier()
	active := rtDestination{domain: "https://unreachable.example.com"}
	inactive := rtDestination{domain: "https://healthy.example.com"}

	s.updateRTStatus(active, 2, &model.CollectorStatus{ActiveClients: 1})
	<-notifier.Wake
	s.updateRTStatus(inactive, 3, &model.CollectorStatus{ActiveClients: 0})
	<-notifier.Wake
	activeUpdatedAt := s.endpointRTStatuses[active].updatedAt
	// Windows can assign the same timestamp to consecutive responses. Keep
	// the healthy status fresh when the unreachable status reaches its TTL.
	inactiveStatus := s.endpointRTStatuses[inactive]
	inactiveStatus.updatedAt = activeUpdatedAt.Add(time.Second)
	s.endpointRTStatuses[inactive] = inactiveStatus

	s.expireRTStatuses(activeUpdatedAt.Add(rtStatusTTL - time.Nanosecond))
	assert.Len(t, notifier.Latest().Statuses, 2)
	s.expireRTStatuses(activeUpdatedAt.Add(rtStatusTTL))
	<-notifier.Wake
	require.Equal(t, []*model.CollectorStatus{{ActiveClients: 0}}, notifier.Latest().Statuses)

	// An expired entry retains its generation guard until fresh feedback arrives.
	s.updateRTStatus(active, 1, &model.CollectorStatus{ActiveClients: 1})
	require.Len(t, notifier.Latest().Statuses, 1)
	s.updateRTStatus(active, 4, &model.CollectorStatus{ActiveClients: 1})
	<-notifier.Wake
	require.Len(t, notifier.Latest().Statuses, 2)
}

// encodeCollectorResponse builds a decodable collector response body.
func encodeCollectorResponse(t *testing.T, activeClients, interval int32) []byte {
	t.Helper()
	out, err := model.EncodeMessage(model.Message{
		Header: model.MessageHeader{
			Version:   model.MessageV3,
			Encoding:  model.MessageEncodingProtobuf,
			Type:      model.MessageType(model.TypeResCollector),
			Timestamp: time.Now().Unix(),
		},
		Body: &model.ResCollector{
			Header: &model.ResCollector_Header{
				Type: model.TypeResCollector,
			},
			Status: &model.CollectorStatus{
				ActiveClients: activeClients,
				Interval:      interval,
			},
		},
	})
	require.NoError(t, err)
	return out
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
	synctest.Test(t, syncTestSubmitterHeartbeatProcess)
}

func syncTestSubmitterHeartbeatProcess(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)
	flavor.SetFlavor(flavor.ProcessAgent)

	ctrl := gomock.NewController(t)
	statsdClient := mockStatsd.NewMockClientInterface(ctrl)
	statsdClient.EXPECT().Gauge("datadog.process.agent", float64(1), gomock.Any(), float64(1)).MinTimes(1)

	deps := getSubmitterDeps(t, nil, nil)
	s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, statsdClient, testHostName, deps.SysProbeConfig)
	assert.NoError(t, err)
	s.Start()
	time.Sleep(15 * time.Second)
	synctest.Wait()
	s.Stop()
}

func TestSubmitterHeartbeatCore(t *testing.T) {
	synctest.Test(t, syncTestSubmitterHeartbeatCore)
}

func syncTestSubmitterHeartbeatCore(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)
	flavor.SetFlavor(flavor.DefaultAgent)

	ctrl := gomock.NewController(t)
	statsdClient := mockStatsd.NewMockClientInterface(ctrl)
	statsdClient.EXPECT().Gauge("datadog.process.agent", float64(1), gomock.Any(), float64(1)).Times(0)

	deps := getSubmitterDeps(t, nil, nil)
	s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, statsdClient, testHostName, deps.SysProbeConfig)
	assert.NoError(t, err)
	s.Start()
	time.Sleep(15 * time.Second)
	synctest.Wait()
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
	sysprobeConf := sysprobeconfigmock.NewMock(t)
	for k, v := range sysprobeconfigOverrides {
		sysprobeConf.SetInTest(k, v)
	}
	return fxutil.Test[submitterDeps](t, fx.Options(
		fx.Provide(func() config.Component { return config.NewMockWithOverrides(t, configOverrides) }),
		fx.Provide(func() sysprobeconfig.Component { return sysprobeConf }),
		connectionsforwarderfx.Module(),
		secretsfxnoop.Module(),
		forwardersimpl.Module(),
		fx.Provide(func() log.Component {
			return logmock.New(t)
		}),
		fx.Provide(func() statsd.ClientInterface {
			return &statsd.NoOpClient{}
		}),
	))
}

func getSubmitterDepsWithConfig(t *testing.T, configObj config.Component) submitterDeps {
	return fxutil.Test[submitterDeps](t, fx.Options(
		fx.Provide(func() config.Component {
			return configObj
		}),
		fx.Provide(func(tb testing.TB) sysprobeconfig.Component { return sysprobeconfigmock.NewMock(tb) }),
		connectionsforwarderfx.Module(),
		secretsfxnoop.Module(),
		forwardersimpl.Module(),
		fx.Provide(func() log.Component {
			return logmock.New(t)
		}),
		fx.Provide(func() statsd.ClientInterface {
			return &statsd.NoOpClient{}
		}),
	))
}
