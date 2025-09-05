/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package syntheticstestschedulerimpl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
	"time"
)

func Test_SyntheticsTestScheduler_StartAndStop(t *testing.T) {
	// GIVEN
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := utillog.LoggerFromWriterWithMinLevelAndFormat(w, utillog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	utillog.SetupLogger(l, "debug")
	configs := &schedulerConfigs{
		workers:                    2,
		flushInterval:              100 * time.Millisecond,
		syntheticsSchedulerEnabled: true,
	}
	scheduler, err := newSyntheticsTestScheduler(configs, nil, l, "configID", time.Now)
	assert.Nil(t, err)
	assert.False(t, scheduler.running)

	// TEST START
	err = scheduler.start()
	assert.Nil(t, err)

	assert.True(t, scheduler.running)

	// TEST START CALLED TWICE
	err = scheduler.start()
	assert.EqualError(t, err, "server already started")

	// TEST STOP
	scheduler.stop()
	assert.False(t, scheduler.running)

	// TEST START/STOP using logs
	l.Close() // We need to first close the logger to avoid a race-cond between seelog and out test when calling w.Flush()
	w.Flush()
	logs := b.String()

	assert.Equal(t, 1, strings.Count(logs, "start Synthetics Test Scheduler"), logs)
	assert.Equal(t, 1, strings.Count(logs, "starting flush loop"), logs)
	assert.Equal(t, 1, strings.Count(logs, "starting workers"), logs)
	assert.Equal(t, 1, strings.Count(logs, "starting worker #0"), logs)

	assert.Equal(t, 1, strings.Count(logs, "stopped flush loop"), logs)
	assert.Equal(t, 1, strings.Count(logs, "all workers stopped"), logs)
	assert.Equal(t, 1, strings.Count(logs, "synthetics test scheduler stopped"), logs)
}

func Test_SyntheticsTestScheduler_OnConfigUpdate(t *testing.T) {
	// GIVEN

	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := utillog.LoggerFromWriterWithMinLevelAndFormat(w, utillog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	utillog.SetupLogger(l, "debug")
	configs := &schedulerConfigs{
		workers:                    2,
		flushInterval:              100 * time.Millisecond,
		syntheticsSchedulerEnabled: true,
	}

	// Table of test cases
	tests := []struct {
		name         string
		updateJSON   map[string]string
		previousJSON map[string]string
	}{{
		name: "no previous config - update with one test",
		updateJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "tcp",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "syn",
							"probe_count": 3,
							"traceroute_count": 1,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"orgID": 12345,
					"mainDC": "us1.staging.dog",
					"publicID": "puf-1"
				}`},
	}, {
		name: "no previous config - update with 2 tests",
		updateJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-2/bbb222": `{
					"version": 1,
					"type": "network",
					"subtype": "udp",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.org",
							"port": 53,
							"probe_count": 2,
							"traceroute_count": 1,
							"max_ttl": 20,
							"timeout": 3,
							"source_service": "api",
							"destination_service": "db"
						}
					},
					"orgID": 67890,
					"mainDC": "us2.staging.dog",
					"publicID": "puf-2"
				}`,
			"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "tcp",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "syn",
							"probe_count": 3,
							"traceroute_count": 1,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"orgID": 12345,
					"mainDC": "us1.staging.dog",
					"publicID": "puf-1"
				}`,
		},
	}, {
		name: "previous config with one test- update with test",
		updateJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "tcp",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "syn",
							"probe_count": 3,
							"traceroute_count": 1,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"orgID": 12345,
					"mainDC": "us1.staging.dog",
					"publicID": "puf-1"
				}`},
		previousJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "tcp",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "sack",
							"probe_count": 3,
							"traceroute_count": 3,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"orgID": 12345,
					"mainDC": "us1.staging.dog",
					"publicID": "puf-1"
				}`},
	}, {
		name: "previous config with one test- add a new  test",
		updateJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-2/bbb222": `{
					"version": 1,
					"type": "network",
					"subtype": "udp",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.org",
							"port": 53,
							"probe_count": 2,
							"traceroute_count": 1,
							"max_ttl": 20,
							"timeout": 3,
							"source_service": "api",
							"destination_service": "db"
						}
					},
					"orgID": 67890,
					"mainDC": "us2.staging.dog",
					"publicID": "puf-2"
				}`,
			"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "tcp",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "syn",
							"probe_count": 3,
							"traceroute_count": 1,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"orgID": 12345,
					"mainDC": "us1.staging.dog",
					"publicID": "puf-1"
				}`},
		previousJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "tcp",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "sack",
							"probe_count": 3,
							"traceroute_count": 3,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"orgID": 12345,
					"mainDC": "us1.staging.dog",
					"publicID": "puf-1"
				}`},
	}, {
		name:       "previous config with one test- delete",
		updateJSON: map[string]string{},
		previousJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
				"version": 1,
				"type": "network",
				"subtype": "tcp",
				"config": {
					"assertions": [],
					"request": {
						"host": "example.com",
						"port": 443,
						"tcp_method": "sack",
						"probe_count": 3,
						"traceroute_count": 3,
						"max_ttl": 30,
						"timeout": 5,
						"source_service": "frontend",
						"destination_service": "backend"
					}
				},
				"orgID": 12345,
				"mainDC": "us1.staging.dog",
				"publicID": "puf-1"
			}`},
	},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("run_path", testDir)
			scheduler, err := newSyntheticsTestScheduler(configs, nil, l, "configID", time.Now)
			assert.Nil(t, err)
			assert.False(t, scheduler.running)
			applied := map[string]state.ApplyStatus{}
			applyFunc := func(id string, status state.ApplyStatus) {
				applied[id] = status
			}

			// Execute previous config
			previousConfigs := map[string]state.RawConfig{}
			for pathID, pConfig := range tt.previousJSON {
				previousConfigs[pathID] = state.RawConfig{Config: []byte(pConfig)}
			}
			scheduler.onConfigUpdate(previousConfigs, func(id string, status state.ApplyStatus) {})

			// Execute update
			configs := map[string]state.RawConfig{}
			expectedApplied := map[string]state.ApplyStatus{}
			for pathID, c := range tt.updateJSON {
				expectedApplied[pathID] = state.ApplyStatus{
					State: state.ApplyStateAcknowledged,
				}
				configs[pathID] = state.RawConfig{Config: []byte(c)}
			}
			scheduler.onConfigUpdate(configs, applyFunc)

			assert.Equal(t, expectedApplied, applied)

			// Verify cache
			cache, err := persistentcache.Read(cacheKey)
			assert.Nil(t, err)

			cfg := map[string]common.SyntheticsTestConfig{}
			for _, v := range tt.updateJSON {
				var newUpdate common.SyntheticsTestConfig
				err = json.Unmarshal([]byte(v), &newUpdate)
				assert.Nil(t, err)
				cfg[newUpdate.PublicID] = newUpdate
			}
			val, err := json.Marshal(cfg)
			assert.Nil(t, err)
			assert.Equal(t, string(val), cache)

			for k := range scheduler.state.tests {
				_, exists := cfg[k]
				assert.True(t, exists)
			}
			for k := range cfg {
				_, exists := scheduler.state.tests[k]
				assert.True(t, exists)
			}
		})

	}
}

func Test_SyntheticsTestScheduler_Processing(t *testing.T) {
	type testCase struct {
		name                  string
		updateJSON            map[string]string
		expectedEventJSON     string
		expectedRunTraceroute func(cfg config.Config, _ telemetry.Component) (payload.NetworkPath, error)
	}

	testCases := []testCase{
		{
			name: "one test provided",
			updateJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
				"version":1,"type":"network","subtype":"tcp",
				"config":{"assertions":[],"request":{"host":"example.com","port":443,"tcp_method":"syn","probe_count":3,"traceroute_count":1,"max_ttl":30,"timeout":5,"source_service":"frontend","destination_service":"backend"}},
				"orgID":12345,"mainDC":"us1.staging.dog","publicID":"puf-9fm-c89"
			}`},
			expectedEventJSON: `{"_dd":{},"result":{"id":"4907739274636687553","initialId":"4907739274636687553","testFinishedAt":1756901488592,"testStartedAt":1756901488591,"testTriggeredAt":1756901488590,"assertions":null,"failure":null,"duration":1,"request":{"host":"example.com","port":443,"maxTtl":30,"timeout":5000},"netstats":{"packetsSent":0,"packetsReceived":0,"packetLossPercentage":0,"jitter":0,"latency":{"avg":0,"min":0,"max":0},"hops":{"avg":0,"min":0,"max":0}},"netpath":{"timestamp":0,"pathtrace_id":"pathtrace-id-111-example.com","origin":"","protocol":"TCP","agent_version":"","namespace":"","source":{"hostname":"abc"},"destination":{"hostname":"example.com","ip_address":"example.com","port":443,"reverse_dns_hostname":""},"hops":[{"ttl":0,"rtt":0,"ip_address":"1.1.1.1","hostname":"hop_1","reachable":false},{"ttl":0,"rtt":0,"ip_address":"1.1.1.2","hostname":"hop_2","reachable":false}],"test_config_id":"configID","test_result_id":"4907739274636687553","traceroute_test":{"traceroute_runs":null,"hop_count_avg":0,"hop_count_min":0,"hop_count_max":0},"e2e_test":{"packet_loss":0,"latency_avg":0,"latency_min":0,"latency_max":0,"jitter":0},"tags":null},"status":"passed"},"test":{"_internalId":"puf-9fm-c89","id":"puf-9fm-c89","subType":"tcp","type":"network","version":1},"v":1}`,
			expectedRunTraceroute: func(cfg config.Config, _ telemetry.Component) (payload.NetworkPath, error) {
				return payload.NetworkPath{
					PathtraceID: "pathtrace-id-111-" + cfg.DestHostname,
					Protocol:    cfg.Protocol,
					Source:      payload.NetworkPathSource{Hostname: "abc"},
					Destination: payload.NetworkPathDestination{Hostname: cfg.DestHostname, IPAddress: cfg.DestHostname, Port: cfg.DestPort},
					Hops: []payload.NetworkPathHop{
						{Hostname: "hop_1", IPAddress: "1.1.1.1"},
						{Hostname: "hop_2", IPAddress: "1.1.1.2"},
					},
				}, nil
			},
		}, {
			name: "two network testsprovided",
			updateJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
				"version":1,"type":"network","subtype":"tcp",
				"config":{"assertions":[],"request":{"host":"example.com","port":443,"tcp_method":"syn","probe_count":3,"traceroute_count":1,"max_ttl":30,"timeout":5,"source_service":"frontend","destination_service":"backend"}},
				"orgID":12345,"mainDC":"us1.staging.dog","publicID":"puf-9fm-c89"
			}`},
			expectedEventJSON: `{"_dd":{},"result":{"id":"4907739274636687553","initialId":"4907739274636687553","testFinishedAt":1756901488592,"testStartedAt":1756901488591,"testTriggeredAt":1756901488590,"assertions":null,"failure":null,"duration":1,"request":{"host":"example.com","port":443,"maxTtl":30,"timeout":5000},"netstats":{"packetsSent":0,"packetsReceived":0,"packetLossPercentage":0,"jitter":0,"latency":{"avg":0,"min":0,"max":0},"hops":{"avg":0,"min":0,"max":0}},"netpath":{"timestamp":0,"pathtrace_id":"pathtrace-id-111-example.com","origin":"","protocol":"TCP","agent_version":"","namespace":"","source":{"hostname":"abc"},"destination":{"hostname":"example.com","ip_address":"example.com","port":443,"reverse_dns_hostname":""},"hops":[{"ttl":0,"rtt":0,"ip_address":"1.1.1.1","hostname":"hop_1","reachable":false},{"ttl":0,"rtt":0,"ip_address":"1.1.1.2","hostname":"hop_2","reachable":false}],"test_config_id":"configID","test_result_id":"4907739274636687553","traceroute_test":{"traceroute_runs":null,"hop_count_avg":0,"hop_count_min":0,"hop_count_max":0},"e2e_test":{"packet_loss":0,"latency_avg":0,"latency_min":0,"latency_max":0,"jitter":0},"tags":null},"status":"passed"},"test":{"_internalId":"puf-9fm-c89","id":"puf-9fm-c89","subType":"tcp","type":"network","version":1},"v":1}`,
			expectedRunTraceroute: func(cfg config.Config, _ telemetry.Component) (payload.NetworkPath, error) {
				return payload.NetworkPath{
					PathtraceID: "pathtrace-id-111-" + cfg.DestHostname,
					Protocol:    cfg.Protocol,
					Source:      payload.NetworkPathSource{Hostname: "abc"},
					Destination: payload.NetworkPathDestination{Hostname: cfg.DestHostname, IPAddress: cfg.DestHostname, Port: cfg.DestPort},
					Hops: []payload.NetworkPathHop{
						{Hostname: "hop_1", IPAddress: "1.1.1.1"},
						{Hostname: "hop_2", IPAddress: "1.1.1.2"},
					},
				}, nil
			},
		},
	}
	configs := &schedulerConfigs{
		workers:                    6,
		flushInterval:              100 * time.Millisecond,
		syntheticsSchedulerEnabled: true,
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testDir := t.TempDir()
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("run_path", testDir)

			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := utillog.LoggerFromWriterWithMinLevelAndFormat(w, utillog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
			assert.Nil(t, err)
			utillog.SetupLogger(l, "debug")

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockEpForwarder := eventplatformimpl.NewMockEventPlatformForwarder(ctrl)

			fixedBase := time.UnixMilli(1756901488589)
			step := 0
			timeNowFn := func() time.Time {
				t := fixedBase.Add(time.Duration(step) * time.Millisecond)
				step++
				return t
			}

			scheduler, err := newSyntheticsTestScheduler(configs, mockEpForwarder, l, "configID", timeNowFn)
			assert.Nil(t, err)
			assert.False(t, scheduler.running)

			configs := map[string]state.RawConfig{}
			expectedApplied := map[string]state.ApplyStatus{}
			for pathID, c := range tc.updateJSON {
				expectedApplied[pathID] = state.ApplyStatus{
					State: state.ApplyStateAcknowledged,
				}
				configs[pathID] = state.RawConfig{Config: []byte(c)}
			}

			scheduler.onConfigUpdate(configs, func(id string, status state.ApplyStatus) {})

			tickCh := make(chan time.Time, 1)
			scheduler.tickerC = tickCh
			tickCh <- scheduler.TimeNowFn()

			scheduler.runTraceroute = tc.expectedRunTraceroute
			scheduler.generateTestResultID = func() (string, error) { return "4907739274636687553", nil }

			var compactJSON bytes.Buffer
			assert.Nil(t, json.Compact(&compactJSON, []byte(tc.expectedEventJSON)))

			done := make(chan struct{})
			mockEpForwarder.EXPECT().
				SendEventPlatformEventBlocking(message.NewMessage(compactJSON.Bytes(), nil, "", 0), eventplatform.EventTypeSynthetics).
				Do(func(_, _ interface{}) { close(done) }).
				Return(nil).Times(1)

			assert.Nil(t, scheduler.start())

			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("mock was never called")
			}
		})
	}
}

func Test_SyntheticsTestScheduler_RunWorker_ProcessesTestCtxAndSendsResult(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := utillog.LoggerFromWriterWithMinLevelAndFormat(w, utillog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	utillog.SetupLogger(l, "debug")

	scheduler := &SyntheticsTestScheduler{
		syntheticsTestProcessingChan: make(chan SyntheticsTestCtx, 1),
		stopChan:                     make(chan struct{}),
		TimeNowFn:                    func() time.Time { return time.Unix(1000, 0) },
		log:                          l,
		flushInterval:                100 * time.Millisecond,
		workers:                      4,
	}

	scheduler.runTraceroute = func(cfg config.Config, _ telemetry.Component) (payload.NetworkPath, error) {
		return payload.NetworkPath{
			PathtraceID: "path-123",
			Protocol:    payload.ProtocolTCP,
			Source:      payload.NetworkPathSource{Hostname: "src"},
			Destination: payload.NetworkPathDestination{Hostname: "dst", Port: 443},
		}, nil
	}

	gotCh := make(chan *WorkerResult, 1)
	scheduler.sendResult = func(w *WorkerResult) error {
		gotCh <- w // signal test that we got a result
		return nil
	}

	testCfg := common.SyntheticsTestConfig{
		Version:  1,
		Type:     "network",
		Subtype:  string(common.SubtypeTCP),
		PublicID: "abc123",
		Interval: 60,
		Config: struct {
			Assertions []interface{}        `json:"assertions"`
			Request    common.ConfigRequest `json:"request"`
		}{
			Request: common.TCPConfigRequest{
				Host:      "dst",
				Port:      ptr(443),
				TCPMethod: common.TCPMethodSYN,
			},
		},
	}

	scheduler.syntheticsTestProcessingChan <- SyntheticsTestCtx{
		nextRun: scheduler.TimeNowFn(),
		cfg:     testCfg,
	}

	go scheduler.runWorker(0)

	var got *WorkerResult
	select {
	case got = <-gotCh:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for WorkerResult")
	}

	close(scheduler.stopChan)

	if got.testCfg.cfg.PublicID != "abc123" {
		t.Errorf("unexpected PublicID: %s", got.testCfg.cfg.PublicID)
	}
	if got.tracerouteResult.PathtraceID != "path-123" {
		t.Errorf("unexpected PathtraceID: %s", got.tracerouteResult.PathtraceID)
	}
}

func TestFlushEnqueuesDueTests(t *testing.T) {
	now := time.Now()
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := utillog.LoggerFromWriterWithMinLevelAndFormat(w, utillog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	utillog.SetupLogger(l, "debug")

	scheduler := &SyntheticsTestScheduler{
		TimeNowFn:                    func() time.Time { return now },
		syntheticsTestProcessingChan: make(chan SyntheticsTestCtx, 10),
		state: runningState{
			tests: map[string]*runningTestState{
				"test1": {
					cfg: common.SyntheticsTestConfig{
						PublicID: "test1",
						Interval: 10, // seconds
					},
					lastRun: now.Add(-20 * time.Second),
					nextRun: now.Add(-10 * time.Second),
				},
			},
		},
		log: l,
	}

	// Flush at 'now'
	scheduler.flush(now)

	select {
	case ctx := <-scheduler.syntheticsTestProcessingChan:
		if ctx.cfg.PublicID != "test1" {
			t.Errorf("expected test1, got %s", ctx.cfg.PublicID)
		}
	default:
		t.Errorf("expected test1 to be enqueued")
	}

	// The nextRun should be updated
	rt := scheduler.state.tests["test1"]
	expectedNextRun := now.Add(time.Duration(rt.cfg.Interval) * time.Second)
	if !rt.nextRun.Equal(expectedNextRun) {
		t.Errorf("expected nextRun %v, got %v", expectedNextRun, rt.nextRun)
	}
}

func ptr[T any](v T) *T { return &v }
