// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package syntheticstestschedulerimpl

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"
)

func Test_SyntheticsTestScheduler_StartAndStop(t *testing.T) {
	// GIVEN
	testDir := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("run_path", testDir)
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := utillog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, utillog.DebugLvl)
	assert.Nil(t, err)
	utillog.SetupLogger(l, "debug")
	configs := &schedulerConfigs{
		workers:                    2,
		flushInterval:              100 * time.Millisecond,
		syntheticsSchedulerEnabled: true,
	}
	scheduler := newSyntheticsTestScheduler(configs, nil, l, nil, time.Now, &teststatsd.Client{}, nil)
	assert.Nil(t, err)
	assert.False(t, scheduler.running)

	ctx := context.TODO()
	// TEST START
	err = scheduler.start(ctx)
	assert.Nil(t, err)

	assert.True(t, scheduler.running)

	// TEST START CALLED TWICE
	err = scheduler.start(ctx)
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
	l, err := utillog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, utillog.DebugLvl)
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
					"subtype": "TCP",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "SYN",
							"probe_count": 3,
							"traceroute_count": 1,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"org_id": 12345,
					"main_dc": "us1.staging.dog",
					"public_id": "puf-1"
				}`},
	}, {
		name: "no previous config - update with 2 tests",
		updateJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-2/bbb222": `{
					"version": 1,
					"type": "network",
					"subtype": "UDP",
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
					"org_id": 67890,
					"main_dc": "us2.staging.dog",
					"public_id": "puf-2"
				}`,
			"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "TCP",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "SYN",
							"probe_count": 3,
							"traceroute_count": 1,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"org_id": 12345,
					"main_dc": "us1.staging.dog",
					"public_id": "puf-1"
				}`,
		},
	}, {
		name: "previous config with one test- update with test",
		updateJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "TCP",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "SYN",
							"probe_count": 3,
							"traceroute_count": 1,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"org_id": 12345,
					"main_dc": "us1.staging.dog",
					"public_id": "puf-1"
				}`},
		previousJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "TCP",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "SACK",
							"probe_count": 3,
							"traceroute_count": 3,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"org_id": 12345,
					"main_dc": "us1.staging.dog",
					"public_id": "puf-1"
				}`},
	}, {
		name: "previous config with one test- add a new  test",
		updateJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-2/bbb222": `{
					"version": 1,
					"type": "network",
					"subtype": "UDP",
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
					"org_id": 67890,
					"main_dc": "us2.staging.dog",
					"public_id": "puf-2"
				}`,
			"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "TCP",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "SYN",
							"probe_count": 3,
							"traceroute_count": 1,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"org_id": 12345,
					"main_dc": "us1.staging.dog",
					"public_id": "puf-1"
				}`},
		previousJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "TCP",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "SACK",
							"probe_count": 3,
							"traceroute_count": 3,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"org_id": 12345,
					"main_dc": "us1.staging.dog",
					"public_id": "puf-1"
				}`},
	}, {
		name:       "previous config with one test- delete",
		updateJSON: map[string]string{},
		previousJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
				"version": 1,
				"type": "network",
				"subtype": "TCP",
				"config": {
					"assertions": [],
					"request": {
						"host": "example.com",
						"port": 443,
						"tcp_method": "SACK",
						"probe_count": 3,
						"traceroute_count": 3,
						"max_ttl": 30,
						"timeout": 5,
						"source_service": "frontend",
						"destination_service": "backend"
					}
				},
				"org_id": 12345,
				"main_dc": "us1.staging.dog",
				"public_id": "puf-1"
			}`},
	}, {
		name: "previous config with one test- update with test with different version",
		updateJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 2,
					"type": "network",
					"subtype": "TCP",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "SYN",
							"probe_count": 3,
							"traceroute_count": 1,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"org_id": 12345,
					"main_dc": "us1.staging.dog",
					"public_id": "puf-1"
				}`},
		previousJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "TCP",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "SACK",
							"probe_count": 3,
							"traceroute_count": 3,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"org_id": 12345,
					"main_dc": "us1.staging.dog",
					"public_id": "puf-1"
				}`},
	}, {
		name: "previous config with one test- update with same version (nextRun should be preserved)",
		updateJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "TCP",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "SYN",
							"probe_count": 3,
							"traceroute_count": 1,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"org_id": 12345,
					"main_dc": "us1.staging.dog",
					"public_id": "puf-1"
				}`},
		previousJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version": 1,
					"type": "network",
					"subtype": "TCP",
					"config": {
						"assertions": [],
						"request": {
							"host": "example.com",
							"port": 443,
							"tcp_method": "SACK",
							"probe_count": 3,
							"traceroute_count": 3,
							"max_ttl": 30,
							"timeout": 5,
							"source_service": "frontend",
							"destination_service": "backend"
						}
					},
					"org_id": 12345,
					"main_dc": "us1.staging.dog",
					"public_id": "puf-1"
				}`},
	},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("run_path", testDir)
			firstUpdateTime := time.Now()
			secondUpdateTime := firstUpdateTime.Add(5 * time.Minute)
			isFirstUpdate := true
			timeNowFn := func() time.Time {
				if isFirstUpdate {
					return firstUpdateTime
				}
				return secondUpdateTime
			}

			scheduler := newSyntheticsTestScheduler(configs, nil, l, nil, timeNowFn, &teststatsd.Client{}, nil)
			assert.False(t, scheduler.running)
			applied := map[string]state.ApplyStatus{}
			applyFunc := func(id string, status state.ApplyStatus) {
				applied[id] = status
			}

			// Execute previous config
			previousConfigs := map[string]state.RawConfig{}
			previousParsedConfigs := map[string]common.SyntheticsTestConfig{}
			for pathID, pConfig := range tt.previousJSON {
				previousConfigs[pathID] = state.RawConfig{Config: []byte(pConfig)}
				var prevCfg common.SyntheticsTestConfig
				err = json.Unmarshal([]byte(pConfig), &prevCfg)
				assert.Nil(t, err)
				previousParsedConfigs[prevCfg.PublicID] = prevCfg
			}
			scheduler.onConfigUpdate(previousConfigs, func(_ string, _ state.ApplyStatus) {})

			// Store the nextRun values after previous config to check if they're preserved
			previousNextRuns := map[string]time.Time{}
			for pubID, state := range scheduler.state.tests {
				previousNextRuns[pubID] = state.nextRun
			}

			// Switch to second update time
			isFirstUpdate = false

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

			// Build expected state based on the logic
			cfg := map[string]*runningTestState{}
			for _, v := range tt.updateJSON {
				var newUpdate common.SyntheticsTestConfig
				err = json.Unmarshal([]byte(v), &newUpdate)
				assert.Nil(t, err)

				expectedNextRun := secondUpdateTime

				// If the test existed before
				if prevCfg, existed := previousParsedConfigs[newUpdate.PublicID]; existed {
					// If version didn't change, nextRun should be preserved
					if newUpdate.Version <= prevCfg.Version {
						expectedNextRun = previousNextRuns[newUpdate.PublicID]
					}
					// If version changed (increased), nextRun should be updated to secondUpdateTime
					// (which is already set above)
				}
				// If it's a new test, nextRun should be secondUpdateTime (already set above)

				cfg[newUpdate.PublicID] = &runningTestState{
					cfg:     newUpdate,
					nextRun: expectedNextRun,
				}
			}

			opts := []cmp.Option{
				cmp.AllowUnexported(runningTestState{}),
			}
			assert.True(t, cmp.Equal(cfg, scheduler.state.tests, opts...), "Diff: %s", cmp.Diff(cfg, scheduler.state.tests, opts...))
		})
	}
}

type tracerouteRunner struct {
	fn func(context.Context, config.Config) (payload.NetworkPath, error)
}

func (t *tracerouteRunner) Run(ctx context.Context, cfg config.Config) (payload.NetworkPath, error) {
	return t.fn(ctx, cfg)
}

func Test_SyntheticsTestScheduler_Processing(t *testing.T) {
	type testCase struct {
		name                  string
		updateJSON            map[string]string
		expectedEventJSON     string
		expectedRunTraceroute func(context.Context, config.Config) (payload.NetworkPath, error)
	}

	testCases := []testCase{
		{
			name: "one test provided",
			updateJSON: map[string]string{"datadog/2/SYNTHETICS_TEST/config-1/aaa111": `{
					"version":1,"type":"network","subtype":"TCP",
					"config":{"assertions":[],"request":{"host":"example.com","port":443,"tcp_method":"SYN","probe_count":3,"traceroute_count":1,"max_ttl":30,"timeout":5,"source_service":"frontend","destination_service":"backend"}},
					"org_id":12345,"main_dc":"us1.staging.dog","public_id":"puf-9fm-c89","run_type":"scheduled"
				}`},
			expectedEventJSON: `{"location":{"id":"agent:test-hostname"},"_dd":{},"result":{"id":"4907739274636687553","initialId":"4907739274636687553","testFinishedAt":1756901488592,"testStartedAt":1756901488591,"testTriggeredAt":1756901488590,"assertions":[],"failure":null,"duration":1,"config":{"assertions":[],"request":{"destinationService":"backend","port":443,"maxTtl":30,"host":"example.com","tracerouteQueries":1,"e2eQueries":3,"sourceService":"frontend","timeout":5,"tcpMethod":"SYN"}},"netstats":{"packetsSent":0,"packetsReceived":0,"packetLossPercentage":0,"jitter":null,"latency":null,"hops":{"avg":0,"min":0,"max":0}},"netpath":{"timestamp":1756901488592,"agent_version":"","namespace":"","test_config_id":"puf-9fm-c89","test_result_id":"4907739274636687553","test_run_id":"test-run-id-111-example.com","origin":"synthetics","test_run_type":"scheduled","source_product":"synthetics","collector_type":"agent","protocol":"TCP","source":{"name":"test-hostname","display_name":"test-hostname","hostname":"test-hostname"},"destination":{"hostname":"example.com","port":443},"traceroute":{"runs":[{"run_id":"1","source":{"ip_address":"","port":0},"destination":{"ip_address":"","port":0},"hops":[{"ttl":0,"ip_address":"1.1.1.1","reachable":false},{"ttl":0,"ip_address":"1.1.1.2","reachable":false}]}],"hop_count":{"avg":0,"min":0,"max":0}},"e2e_probe":{"rtts":null,"packets_sent":0,"packets_received":0,"packet_loss_percentage":0,"jitter":0,"rtt":{"avg":0,"min":0,"max":0}}},"status":"passed","runType":"scheduled"},"test":{"id":"puf-9fm-c89","subType":"tcp","type":"network","version":1},"v":1}`,
			expectedRunTraceroute: func(_ context.Context, cfg config.Config) (payload.NetworkPath, error) {
				return payload.NetworkPath{
					TestRunID:   "test-run-id-111-" + cfg.DestHostname,
					Protocol:    cfg.Protocol,
					Destination: payload.NetworkPathDestination{Hostname: cfg.DestHostname, Port: cfg.DestPort},
					Traceroute: payload.Traceroute{
						Runs: []payload.TracerouteRun{{
							RunID: "1",
							Hops: []payload.TracerouteHop{
								{IPAddress: net.ParseIP("1.1.1.1")},
								{IPAddress: net.ParseIP("1.1.1.2")},
							},
						}},
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
			l, err := utillog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, utillog.DebugLvl)
			assert.Nil(t, err)
			utillog.SetupLogger(l, "debug")

			fixedBase := time.UnixMilli(1756901488589)
			step := 0
			timeNowFn := func() time.Time {
				t := fixedBase.Add(time.Duration(step) * time.Millisecond)
				step++
				return t
			}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockEpForwarder := eventplatformimpl.NewMockEventPlatformForwarder(ctrl)

			ctx := context.TODO()
			scheduler := newSyntheticsTestScheduler(configs, mockEpForwarder, l, &mockHostname{}, timeNowFn, &teststatsd.Client{}, &tracerouteRunner{tc.expectedRunTraceroute})
			assert.False(t, scheduler.running)

			configs := map[string]state.RawConfig{}
			expectedApplied := map[string]state.ApplyStatus{}
			for pathID, c := range tc.updateJSON {
				expectedApplied[pathID] = state.ApplyStatus{
					State: state.ApplyStateAcknowledged,
				}
				configs[pathID] = state.RawConfig{Config: []byte(c)}
			}

			scheduler.onConfigUpdate(configs, func(_ string, _ state.ApplyStatus) {})

			tickCh := make(chan time.Time, 1)
			scheduler.tickerC = tickCh
			tickCh <- scheduler.timeNowFn()

			scheduler.generateTestResultID = func(func(rand io.Reader, max *big.Int) (n *big.Int, err error)) (string, error) {
				return "4907739274636687553", nil
			}

			var compactJSON bytes.Buffer
			assert.Nil(t, json.Compact(&compactJSON, []byte(tc.expectedEventJSON)))

			done := make(chan struct{})
			mockEpForwarder.EXPECT().
				SendEventPlatformEventBlocking(message.NewMessage(compactJSON.Bytes(), nil, "", 0), eventplatform.EventTypeSynthetics).
				Do(func(_, _ interface{}) { close(done) }).
				Return(nil).Times(1)

			assert.Nil(t, scheduler.start(ctx))

			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("mock was never called")
			}
			scheduler.stop()

		})
	}
}

type mockHostname struct{}

func (m *mockHostname) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{
		Hostname: "test-hostname",
		Provider: "test-provider",
	}, nil
}
func (m *mockHostname) GetSafe(_ context.Context) string {
	return "test-hostname"
}
func (m *mockHostname) Get(_ context.Context) (string, error) {
	return "test-hostname", nil
}

func Test_SyntheticsTestScheduler_RunWorker_ProcessesTestCtxAndSendsResult(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := utillog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, utillog.DebugLvl)
	assert.Nil(t, err)
	utillog.SetupLogger(l, "debug")
	ctx, cancel := context.WithCancel(context.TODO())

	scheduler := &syntheticsTestScheduler{
		syntheticsTestProcessingChan: make(chan SyntheticsTestCtx, 1),
		cancel:                       cancel,
		timeNowFn:                    func() time.Time { return time.Unix(1000, 0) },
		log:                          l,
		flushInterval:                100 * time.Millisecond,
		workers:                      4,
		hostNameService:              &mockHostname{},
		statsdClient:                 &teststatsd.Client{},
		traceroute: &tracerouteRunner{fn: func(context.Context, config.Config) (payload.NetworkPath, error) {
			return payload.NetworkPath{
				TestRunID:   "path-123",
				Protocol:    payload.ProtocolTCP,
				Source:      payload.NetworkPathSource{Hostname: "src"},
				Destination: payload.NetworkPathDestination{Hostname: "dst", Port: 443},
			}, nil
		}},
	}

	gotCh := make(chan *workerResult, 1)
	scheduler.sendResult = func(w *workerResult) (string, error) {
		gotCh <- w // signal test that we got a result
		return "", nil
	}

	testCfg := common.SyntheticsTestConfig{
		Version:  1,
		Type:     "network",
		PublicID: "abc123",
		Interval: 60,
		Config: struct {
			Assertions []common.Assertion   `json:"assertions"`
			Request    common.ConfigRequest `json:"request"`
		}{
			Request: common.TCPConfigRequest{
				Host:      "dst",
				Port:      ptr(443),
				TCPMethod: payload.TCPConfigSYN,
			},
		},
	}

	scheduler.syntheticsTestProcessingChan <- SyntheticsTestCtx{
		nextRun: scheduler.timeNowFn(),
		cfg:     testCfg,
	}

	go scheduler.runWorker(ctx, 0)

	var got *workerResult
	select {
	case got = <-gotCh:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for workerResult")
	}

	scheduler.stop()

	if got.testCfg.cfg.PublicID != "abc123" {
		t.Errorf("unexpected PublicID: %s", got.testCfg.cfg.PublicID)
	}
	if got.tracerouteResult.TestRunID != "path-123" {
		t.Errorf("unexpected TestRunID: %s", got.tracerouteResult.TestRunID)
	}
}
func TestFlushEnqueuesDueTests(t *testing.T) {
	now := time.Now()
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := utillog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, utillog.DebugLvl)
	assert.Nil(t, err)
	utillog.SetupLogger(l, "debug")

	scheduler := &syntheticsTestScheduler{
		timeNowFn:                    func() time.Time { return now },
		syntheticsTestProcessingChan: make(chan SyntheticsTestCtx, 10),
		running:                      true,
		state: runningState{
			tests: map[string]*runningTestState{
				"test1": {
					cfg: common.SyntheticsTestConfig{
						PublicID: "test1",
						Interval: 10, // seconds
					},
					nextRun: now.Add(-10 * time.Second),
				},
			},
		},
		flushInterval: 10 * time.Second,
		log:           l,
	}

	// Flush at 'now'
	scheduler.flush(context.Background(), now)

	select {
	case ctx := <-scheduler.syntheticsTestProcessingChan:
		time.Sleep(500 * time.Millisecond)
		if ctx.cfg.PublicID != "test1" {
			t.Errorf("expected test1, got %s", ctx.cfg.PublicID)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("expected test1 to be enqueued")
	}

	rt := scheduler.state.tests["test1"]

	// The nextRun should be updated based on the old nextRun, not flushTime
	expectedNextRun := now // old nextRun (-10s) + interval (10s) = now
	assert.Equal(t, expectedNextRun, rt.nextRun)
}

func TestFlushEnqueueExhaustion(t *testing.T) {
	now := time.Now()
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := utillog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, utillog.DebugLvl)
	assert.Nil(t, err)
	tl := &testLogger{
		LoggerInterface: l,
	}
	utillog.SetupLogger(tl, "debug")

	scheduler := &syntheticsTestScheduler{
		timeNowFn:                    func() time.Time { return now },
		syntheticsTestProcessingChan: make(chan SyntheticsTestCtx, 1),
		running:                      true,
		state: runningState{
			tests: map[string]*runningTestState{
				"test1": {
					cfg: common.SyntheticsTestConfig{
						PublicID: "test1",
						Interval: 10, // seconds
					},
					nextRun: now.Add(-10 * time.Second),
				},
				"test2": {
					cfg: common.SyntheticsTestConfig{
						PublicID: "test1",
						Interval: 10, // seconds
					},
					nextRun: now.Add(-10 * time.Second),
				},
			},
		},
		flushInterval: 100 * time.Millisecond,
		log:           tl,
	}

	// Flush at 'now'
	scheduler.flush(context.Background(), now)

	select {
	case ctx := <-scheduler.syntheticsTestProcessingChan:
		time.Sleep(200 * time.Millisecond)
		if ctx.cfg.PublicID != "test1" {
			t.Errorf("expected test1, got %s", ctx.cfg.PublicID)
		}
	case <-time.After(300 * time.Millisecond):
		t.Errorf("expected test2 not to be enqueued")
	}

	assert.Equal(t, []string{"test queue high usage (≥70%), increase the number of workers", "enqueuing test test1 timed out, increase the number of workers"}, tl.errorCalls)
	rt := scheduler.state.tests["test1"]

	// The nextRun should be updated based on the old nextRun, not flushTime
	expectedNextRun := now // old nextRun (-10s) + interval (10s) = now
	assert.Equal(t, expectedNextRun, rt.nextRun)
}

type testLogger struct {
	utillog.LoggerInterface
	errorCalls []string
}

func (l *testLogger) Warnf(format string, params ...interface{}) error {
	l.errorCalls = append(l.errorCalls, fmt.Sprintf(format, params...))
	return nil
}
func ptr[T any](v T) *T { return &v }
