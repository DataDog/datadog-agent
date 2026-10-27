// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package runner

import (
	"errors"
	"expvar"
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
	"github.com/DataDog/datadog-agent/pkg/process/status"
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

func TestSubmitterDeliversNextPayloadAndRTBeforeSlowDomain(t *testing.T) {
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

		submitFn := func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			calls.Add(1)

			responses := make(chan forwarder.Response, 1)
			responses <- forwarder.Response{
				Domain:     "https://healthy.example.com",
				StatusCode: http.StatusAccepted,
				Body:       healthyBody,
			}
			// The additional endpoint never replies until the test releases it.
			go func() {
				<-release
				close(responses)
			}()
			return responses, nil
		}
		s.submitFuncs[checks.ProcessCheckName] = submitFn

		require.NoError(t, s.Start())

		submitProcessPayload(s)
		synctest.Wait()
		require.EqualValues(t, 1, calls.Load(), "first payload was never submitted")
		assert.Equal(t, int32(1), rtStatuses(t, s)[0].ActiveClients, "healthy endpoint should enable RT before the slow endpoint replies")

		submitProcessPayload(s)
		synctest.Wait()
		require.EqualValues(t, 2, calls.Load(), "second payload must not wait for the first payload's slow endpoint")
		assert.Equal(t, int32(1), rtStatuses(t, s)[0].ActiveClients)
	})
}

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

func TestSubmitterAggregatesRTAsResponsesArrive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, map[string]interface{}{
			"process_config.process_dd_url":       "https://shared.example.com",
			"process_config.additional_endpoints": map[string][]string{"https://shared.example.com": {"second-key"}},
		}, nil)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		t.Cleanup(s.Stop)

		responses := make(chan forwarder.Response, 2)
		responses <- forwarder.Response{Domain: "https://shared.example.com", StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 1, 2)}
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			return responses, nil
		}
		require.NoError(t, s.Start())
		submitProcessPayload(s)
		synctest.Wait()

		responses <- forwarder.Response{Domain: "https://shared.example.com", APIKeyIndex: 1, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 0, 10)}
		close(responses)
		// The runner can still be processing the first status when the second arrives.
		// Its next update must include both API keys at this domain.
		synctest.Wait()
		combined := rtStatuses(t, s)
		require.Len(t, combined, 2)
		assert.ElementsMatch(t, []int32{1, 0}, []int32{combined[0].ActiveClients, combined[1].ActiveClients})
		runner, err := NewRunnerWithChecks(deps.Config, nil, nil, nil, true, nil)
		require.NoError(t, err)
		runner.UpdateRTStatus(combined)
		assert.True(t, runner.IsRealTimeEnabled())
		assert.Equal(t, 10*time.Second, runner.realTimeInterval)
	})
}

func TestSubmitterIgnoresStaleRTResponses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		status.InitExpvars(deps.Config, testHostName, false, false, nil)
		before := submissionErrorCount(t)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		t.Cleanup(s.Stop)

		first := make(chan forwarder.Response, 2)
		second := make(chan forwarder.Response, 1)
		var calls int
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			calls++
			if calls == 1 {
				return first, nil
			}
			return second, nil
		}
		require.NoError(t, s.Start())
		submitProcessPayload(s)
		submitProcessPayload(s)
		synctest.Wait()
		require.Equal(t, 2, calls)
		second <- forwarder.Response{StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 1, 5)}
		close(second)
		synctest.Wait()
		assert.Equal(t, int32(1), rtStatuses(t, s)[0].ActiveClients)

		first <- forwarder.Response{Domain: "stale.example.com", Err: errors.New("connection refused")}
		first <- forwarder.Response{StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 0, 30)}
		close(first)
		synctest.Wait()
		select {
		case <-s.GetRTNotifierChan():
			t.Fatal("an older response overwrote the newer RT decision")
		default:
		}
		assert.Equal(t, before+1, submissionErrorCount(t), "stale RT responses must still account for delivery errors")
	})
}

func TestSubmitterOrdersConcurrentQueuesBeforeSubmitReturns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		started := make(chan struct{})
		release := make(chan struct{})
		releaseOnce := sync.OnceFunc(func() { close(release) })
		t.Cleanup(func() {
			releaseOnce()
			s.Stop()
		})

		const domain = "https://shared.example.com"
		older := make(chan forwarder.Response, 1)
		older <- forwarder.Response{Domain: domain, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 0, 2)}
		close(older)
		newer := make(chan forwarder.Response, 1)
		newer <- forwarder.Response{Domain: domain, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 1, 2)}
		close(newer)
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			close(started)
			<-release // Older submission must not become newer by returning last.
			return older, nil
		}
		s.submitFuncs[checks.RTProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			return newer, nil
		}
		require.NoError(t, s.Start())

		submitProcessPayload(s)
		<-started
		s.Submit(time.Now(), checks.RTProcessCheckName, &types.Payload{
			CheckName: checks.RTProcessCheckName,
			Message:   []model.MessageBody{&model.CollectorRealTime{HostName: testHostName}},
		})
		synctest.Wait()
		assert.Equal(t, int32(1), rtStatuses(t, s)[0].ActiveClients)

		releaseOnce()
		synctest.Wait()
		select {
		case <-s.GetRTNotifierChan():
			t.Fatal("older standard response overwrote newer RT status")
		default:
		}
	})
}

func TestSubmitterRetainsSlowEndpointRTAcrossSubmissions(t *testing.T) {
	for _, tc := range []struct {
		name         string
		slowDomain   string
		slowKeyIndex uint
		config       map[string]interface{}
	}{
		{name: "different domain", slowDomain: "https://slow.example.com"},
		{
			name: "same domain, different API key", slowDomain: "https://fast.example.com", slowKeyIndex: 1,
			config: map[string]interface{}{
				"process_config.process_dd_url":       "https://fast.example.com",
				"process_config.additional_endpoints": map[string][]string{"https://fast.example.com": {"second-key"}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				deps := getSubmitterDeps(t, tc.config, nil)
				s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
				require.NoError(t, err)
				t.Cleanup(s.Stop)
				runner, err := NewRunnerWithChecks(deps.Config, nil, nil, nil, true, nil)
				require.NoError(t, err)

				first := make(chan forwarder.Response, 2)
				second := make(chan forwarder.Response, 2)
				third := make(chan forwarder.Response, 1)
				channels := []chan forwarder.Response{first, second, third}
				var calls int
				s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
					channel := channels[calls]
					calls++
					return channel, nil
				}
				require.NoError(t, s.Start())

				fast := forwarder.Response{Domain: "https://fast.example.com", StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 0, 2)}
				submitProcessPayload(s)
				first <- fast
				synctest.Wait()
				runner.UpdateRTStatus(rtStatuses(t, s))
				assert.False(t, runner.IsRealTimeEnabled())

				submitProcessPayload(s)
				second <- fast
				synctest.Wait()
				_ = rtStatuses(t, s)

				// This response is older than the second fast response, but is
				// still the newest decision from its own endpoint/key.
				first <- forwarder.Response{Domain: tc.slowDomain, APIKeyIndex: tc.slowKeyIndex, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 1, 10)}
				synctest.Wait()
				statuses := rtStatuses(t, s)
				require.Len(t, statuses, 2)
				runner.UpdateRTStatus(statuses)
				assert.True(t, runner.IsRealTimeEnabled())
				assert.Equal(t, 10*time.Second, runner.realTimeInterval)

				submitProcessPayload(s)
				third <- fast
				synctest.Wait()
				runner.UpdateRTStatus(rtStatuses(t, s))
				assert.True(t, runner.IsRealTimeEnabled(), "a new fast response must retain the slow endpoint's decision")
				assert.Equal(t, 10*time.Second, runner.realTimeInterval)

				second <- forwarder.Response{Domain: tc.slowDomain, APIKeyIndex: tc.slowKeyIndex, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 0, 2)}
				synctest.Wait()
				runner.UpdateRTStatus(rtStatuses(t, s))
				assert.False(t, runner.IsRealTimeEnabled())
				assert.Equal(t, 2*time.Second, runner.realTimeInterval)
			})
		})
	}
}

func TestSubmitterSharedDomainErrorsDoNotGrowRT(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, map[string]interface{}{
			"process_config.process_dd_url":       "https://shared.example.com",
			"process_config.additional_endpoints": map[string][]string{"https://shared.example.com": {"second-key"}},
		}, nil)
		status.InitExpvars(deps.Config, testHostName, false, false, nil)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		t.Cleanup(s.Stop)
		runner, err := NewRunnerWithChecks(deps.Config, nil, nil, nil, true, nil)
		require.NoError(t, err)
		channels := []chan forwarder.Response{
			make(chan forwarder.Response, 2),
			make(chan forwarder.Response, 2),
			make(chan forwarder.Response, 2),
		}
		var calls int
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			channel := channels[calls]
			calls++
			return channel, nil
		}
		require.NoError(t, s.Start())

		const domain = "https://shared.example.com"
		response := func(keyIndex uint, clients int32) forwarder.Response {
			return forwarder.Response{Domain: domain, APIKeyIndex: keyIndex, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, clients, 2)}
		}
		errorResponse := forwarder.Response{Domain: domain, APIKeyIndex: 1, Err: errors.New("revoked API key")}
		channels[0] <- response(0, 1)
		channels[0] <- response(1, 0)
		close(channels[0])
		submitProcessPayload(s)
		synctest.Wait()
		initial := rtStatuses(t, s)
		require.Len(t, initial, 2)
		runner.UpdateRTStatus(initial)
		assert.True(t, runner.IsRealTimeEnabled())

		// An error before a success must replace, not append to, the old aggregate.
		channels[1] <- errorResponse
		channels[1] <- response(0, 0)
		close(channels[1])
		submitProcessPayload(s)
		synctest.Wait()
		inactive := rtStatuses(t, s)
		require.Len(t, inactive, 2)
		runner.UpdateRTStatus(inactive)
		assert.False(t, runner.IsRealTimeEnabled())

		channels[2] <- errorResponse
		channels[2] <- response(0, 1)
		close(channels[2])
		submitProcessPayload(s)
		synctest.Wait()
		active := rtStatuses(t, s)
		require.Len(t, active, 2)
		runner.UpdateRTStatus(active)
		assert.True(t, runner.IsRealTimeEnabled())
	})
}

func TestSubmitterKeepsRTAfterTransientResponseError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		status.InitExpvars(deps.Config, testHostName, false, false, nil)
		before := submissionErrorCount(t)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		t.Cleanup(s.Stop)
		channels := []chan forwarder.Response{
			make(chan forwarder.Response, 1),
			make(chan forwarder.Response, 1),
			make(chan forwarder.Response, 1),
		}
		var calls int
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			channel := channels[calls]
			calls++
			return channel, nil
		}
		require.NoError(t, s.Start())

		const domain = "https://healthy.example.com"
		channels[0] <- forwarder.Response{Domain: domain, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 1, 2)}
		close(channels[0])
		submitProcessPayload(s)
		synctest.Wait()
		assert.Equal(t, int32(1), rtStatuses(t, s)[0].ActiveClients)

		channels[1] <- forwarder.Response{Domain: domain, Err: errors.New("temporary network error")}
		close(channels[1])
		submitProcessPayload(s)
		synctest.Wait()
		select {
		case <-s.GetRTNotifierChan():
			t.Fatal("one failed response disabled real-time collection")
		default:
		}
		assert.Equal(t, before+1, submissionErrorCount(t))

		channels[2] <- forwarder.Response{Domain: domain, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 0, 2)}
		close(channels[2])
		submitProcessPayload(s)
		synctest.Wait()
		assert.Equal(t, int32(0), rtStatuses(t, s)[0].ActiveClients)
	})
}

func TestSubmitterExpiresUnansweredRTEndpoint(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		t.Cleanup(s.Stop)
		runner, err := NewRunnerWithChecks(deps.Config, nil, nil, nil, true, nil)
		require.NoError(t, err)

		const rapidFailures = 4
		channels := make([]chan forwarder.Response, rapidFailures+2)
		channels[0] = make(chan forwarder.Response, 1)
		channels[0] <- forwarder.Response{Domain: "https://healthy.example.com", StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 1, 2)}
		close(channels[0])
		for i := 1; i < len(channels); i++ {
			channels[i] = make(chan forwarder.Response)
			close(channels[i]) // The forwarder timed out without an answer.
		}
		calls := 0
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			channel := channels[calls]
			calls++
			return channel, nil
		}
		require.NoError(t, s.Start())
		submitProcessPayload(s)
		synctest.Wait()
		runner.UpdateRTStatus(rtStatuses(t, s))
		assert.True(t, runner.IsRealTimeEnabled())

		for i := 1; i < len(channels); i++ {
			if i == rapidFailures+1 {
				time.Sleep(rtStatusFailureGracePeriod + time.Nanosecond)
			}
			submitProcessPayload(s)
			synctest.Wait()
			if i <= rapidFailures {
				select {
				case <-s.GetRTNotifierChan():
					t.Fatal("a short gap in responses disabled real-time collection")
				default:
				}
				assert.True(t, runner.IsRealTimeEnabled())
				continue
			}
			runner.UpdateRTStatus(rtStatuses(t, s))
			assert.False(t, runner.IsRealTimeEnabled())
		}
	})
}

func TestSubmitterExpiresRepeatedRTFailures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		status.InitExpvars(deps.Config, testHostName, false, false, nil)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		t.Cleanup(s.Stop)
		runner, err := NewRunnerWithChecks(deps.Config, nil, nil, nil, true, nil)
		require.NoError(t, err)
		const rapidFailures = 4
		channels := make([]chan forwarder.Response, rapidFailures+2)
		for i := range channels {
			channels[i] = make(chan forwarder.Response, 2)
		}
		calls := 0
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			channel := channels[calls]
			calls++
			return channel, nil
		}
		require.NoError(t, s.Start())

		fast := forwarder.Response{Domain: "https://fast.example.com", StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 0, 2)}
		slow := forwarder.Response{Domain: "https://slow.example.com", StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 1, 10)}
		channels[0] <- fast
		channels[0] <- slow
		close(channels[0])
		submitProcessPayload(s)
		synctest.Wait()
		runner.UpdateRTStatus(rtStatuses(t, s))
		assert.True(t, runner.IsRealTimeEnabled())
		assert.Equal(t, 10*time.Second, runner.realTimeInterval)

		for i := 1; i < len(channels); i++ {
			if i == rapidFailures+1 {
				time.Sleep(rtStatusFailureGracePeriod + time.Nanosecond)
			}
			channels[i] <- fast
			channels[i] <- forwarder.Response{Domain: slow.Domain, Err: errors.New("persistent failure")}
			close(channels[i])
			submitProcessPayload(s)
			synctest.Wait()
			runner.UpdateRTStatus(rtStatuses(t, s))
			assert.Equal(t, i <= rapidFailures, runner.IsRealTimeEnabled())
		}
		assert.Equal(t, 2*time.Second, runner.realTimeInterval)
	})
}

func TestSubmitterInvalidatesReorderedKeyVotes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		t.Cleanup(s.Stop)

		const domain = "https://shared.example.com"
		initial := make(chan forwarder.Response, 2)
		initial <- forwarder.Response{Domain: domain, APIKeyIndex: 1, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 0, 2)}
		initial <- forwarder.Response{Domain: domain, APIKeyIndex: 2, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 1, 10)}
		close(initial)
		rotated := make(chan forwarder.Response, 1)
		rotated <- forwarder.Response{Domain: domain, APIKeyIndex: 1, APIKeyVersion: 1, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 0, 2)}
		calls := 0
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			calls++
			if calls == 1 {
				return initial, nil
			}
			return rotated, nil
		}
		require.NoError(t, s.Start())

		submitProcessPayload(s)
		synctest.Wait()
		original := rtStatuses(t, s)
		require.Len(t, original, 2)
		assert.Equal(t, int32(1), original[0].ActiveClients+original[1].ActiveClients)

		// Index 2 no longer names its former key after the domain's keys reorder.
		submitProcessPayload(s)
		synctest.Wait()
		current := rtStatuses(t, s)
		require.Len(t, current, 1, "old key-index votes must not survive a credential-set change")
		assert.Equal(t, int32(0), current[0].ActiveClients)
	})
}

func TestSubmitterInvalidatesDomainVotesDuringRotation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		status.InitExpvars(deps.Config, testHostName, false, false, nil)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		t.Cleanup(s.Stop)
		runner, err := NewRunnerWithChecks(deps.Config, nil, nil, nil, true, nil)
		require.NoError(t, err)

		first := make(chan forwarder.Response, 2)
		second := make(chan forwarder.Response, 3)
		calls := 0
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			calls++
			if calls == 1 {
				return first, nil
			}
			return second, nil
		}
		require.NoError(t, s.Start())
		const domain = "https://shared.example.com"
		first <- forwarder.Response{Domain: domain, APIKeyIndex: 1, APIKeyVersion: 0, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 1, 10)}
		submitProcessPayload(s)
		synctest.Wait()
		runner.UpdateRTStatus(rtStatuses(t, s))
		assert.True(t, runner.IsRealTimeEnabled())

		second <- forwarder.Response{Domain: domain, APIKeyIndex: 0, APIKeyVersion: 1, Err: errors.New("rotated key failed")}
		submitProcessPayload(s)
		synctest.Wait()
		runner.UpdateRTStatus(rtStatuses(t, s))
		assert.False(t, runner.IsRealTimeEnabled(), "old key-index votes must be invalidated on rotation")

		second <- forwarder.Response{Domain: domain, APIKeyIndex: 0, APIKeyVersion: 1, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 0, 2)}
		synctest.Wait()
		runner.UpdateRTStatus(rtStatuses(t, s))
		assert.False(t, runner.IsRealTimeEnabled())
		assert.Equal(t, 2*time.Second, runner.realTimeInterval)

		first <- forwarder.Response{Domain: domain, APIKeyIndex: 1, APIKeyVersion: 0, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 0, 2)}
		synctest.Wait()
		select {
		case <-s.GetRTNotifierChan():
			t.Fatal("a delayed pre-rotation response changed RT")
		default:
		}
		second <- forwarder.Response{Domain: domain, APIKeyIndex: 1, APIKeyVersion: 1, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 1, 10)}
		close(second)
		synctest.Wait()
		current := rtStatuses(t, s)
		require.Len(t, current, 2)
		runner.UpdateRTStatus(current)
		assert.True(t, runner.IsRealTimeEnabled())
		assert.Equal(t, 10*time.Second, runner.realTimeInterval)
	})
}

func TestSubmitterDoesNotCountOldChannelCloseAfterRotation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		t.Cleanup(s.Stop)

		const domain = "https://shared.example.com"
		initial := make(chan forwarder.Response, 1)
		initial <- forwarder.Response{Domain: domain, APIKeyVersion: 0, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 1, 2)}
		close(initial)
		oldSubmissionStarted := make(chan struct{})
		releaseOldSubmission := make(chan struct{})
		t.Cleanup(func() {
			select {
			case <-releaseOldSubmission:
			default:
				close(releaseOldSubmission)
			}
		})
		oldResponse := make(chan forwarder.Response)
		close(oldResponse)
		processCalls := 0
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			processCalls++
			if processCalls == 1 {
				return initial, nil
			}
			close(oldSubmissionStarted)
			<-releaseOldSubmission // Submitted under the old key but returns after rotation.
			return oldResponse, nil
		}

		rotated := make([]chan forwarder.Response, 2)
		rotated[0] = make(chan forwarder.Response, 1)
		rotated[0] <- forwarder.Response{Domain: domain, APIKeyVersion: 1, StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 1, 2)}
		close(rotated[0])
		for i := 1; i < len(rotated); i++ {
			rotated[i] = make(chan forwarder.Response, 1)
			rotated[i] <- forwarder.Response{Domain: domain, APIKeyVersion: 1, Err: errors.New("transient failure")}
			close(rotated[i])
		}
		rtCalls := 0
		s.submitFuncs[checks.RTProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			channel := rotated[rtCalls]
			rtCalls++
			return channel, nil
		}

		require.NoError(t, s.Start())
		submitProcessPayload(s)
		synctest.Wait()
		assert.Equal(t, int32(1), rtStatuses(t, s)[0].ActiveClients)

		submitProcessPayload(s)
		<-oldSubmissionStarted
		for i := range rotated {
			s.Submit(time.Now(), checks.RTProcessCheckName, &types.Payload{
				CheckName: checks.RTProcessCheckName,
				Message:   []model.MessageBody{&model.CollectorRealTime{HostName: testHostName}},
			})
			synctest.Wait()
			if i == 0 {
				assert.Equal(t, int32(1), rtStatuses(t, s)[0].ActiveClients)
			}
		}
		time.Sleep(rtStatusFailureGracePeriod + time.Nanosecond)
		close(releaseOldSubmission)
		synctest.Wait()
		select {
		case <-s.GetRTNotifierChan():
			t.Fatal("an old channel's closure erased the rotated credential's vote")
		default:
		}
	})
}

func TestSubmitterCountsResponseErrorsWhileAnotherDomainSucceeds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		status.InitExpvars(deps.Config, testHostName, false, false, nil)
		before := submissionErrorCount(t)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		t.Cleanup(s.Stop)

		responses := make(chan forwarder.Response, 3)
		responses <- forwarder.Response{Domain: "bad.example.com", Err: errors.New("connection refused")}
		responses <- forwarder.Response{Domain: "bad.example.com", StatusCode: http.StatusInternalServerError}
		responses <- forwarder.Response{Domain: "healthy.example.com", StatusCode: http.StatusAccepted, Body: encodeCollectorResponse(t, 1, 2)}
		close(responses)
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			return responses, nil
		}
		require.NoError(t, s.Start())
		submitProcessPayload(s)
		synctest.Wait()
		assert.Equal(t, int32(1), rtStatuses(t, s)[0].ActiveClients)
		assert.Equal(t, before+2, submissionErrorCount(t))
	})
}

func TestSubmitterCountsImmediateSubmissionErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		status.InitExpvars(deps.Config, testHostName, false, false, nil)
		before := submissionErrorCount(t)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		t.Cleanup(s.Stop)
		var calls atomic.Int32
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			calls.Add(1)
			return make(chan forwarder.Response), errors.New("forwarder stopped")
		}
		require.NoError(t, s.Start())
		const failedSubmissions = 3
		for range failedSubmissions {
			submitProcessPayload(s)
			synctest.Wait()
		}
		assert.EqualValues(t, failedSubmissions, calls.Load())
		assert.Equal(t, before+int64(failedSubmissions), submissionErrorCount(t))
	})
}

func TestSubmitterContinuesPast256PendingResponsesAndStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := getSubmitterDeps(t, nil, nil)
		status.InitExpvars(deps.Config, testHostName, false, false, nil)
		before := submissionErrorCount(t)
		s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, testHostName, deps.SysProbeConfig)
		require.NoError(t, err)
		stopOnce := sync.OnceFunc(s.Stop)
		t.Cleanup(stopOnce)
		const pending = 257
		var calls atomic.Int32
		inactiveBody := encodeCollectorResponse(t, 0, 2)
		activeBody := encodeCollectorResponse(t, 1, 2)
		s.submitFuncs[checks.ProcessCheckName] = func(_ transaction.BytesPayloads, _ http.Header) (chan forwarder.Response, error) {
			call := calls.Add(1)
			body := inactiveBody
			if call == pending {
				body = activeBody
			}
			responses := make(chan forwarder.Response, 1)
			responses <- forwarder.Response{Domain: "https://healthy.example.com", StatusCode: http.StatusAccepted, Body: body}
			return responses, nil // The unreachable endpoint never closes this channel.
		}
		require.NoError(t, s.Start())
		for range pending {
			submitProcessPayload(s)
			synctest.Wait()
		}
		require.EqualValues(t, pending, calls.Load(), "unreachable endpoint blocked healthy submissions")
		assert.Equal(t, int32(1), rtStatuses(t, s)[0].ActiveClients, "latest healthy response must still update RT")
		assert.Equal(t, before, submissionErrorCount(t))

		// Stop must join all drains without waiting for a forwarder channel to close.
		stopOnce()
	})
}

func submissionErrorCount(t *testing.T) int64 {
	t.Helper()
	count := expvar.Get("process_agent").(*expvar.Map).Get("submission_error_count")
	require.NotNil(t, count)
	value, err := strconv.ParseInt(count.String(), 10, 64)
	require.NoError(t, err)
	return value
}

func submitProcessPayload(s *CheckSubmitter) {
	s.Submit(time.Now(), checks.ProcessCheckName, &types.Payload{
		CheckName: checks.ProcessCheckName,
		Message:   []model.MessageBody{&model.CollectorProc{HostName: testHostName}},
	})
}

func rtStatuses(t *testing.T, s *CheckSubmitter) types.RTResponse {
	t.Helper()
	select {
	case statuses := <-s.GetRTNotifierChan():
		return statuses
	default:
		t.Fatal("expected an RT status without waiting for the slow endpoint")
		return nil
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
