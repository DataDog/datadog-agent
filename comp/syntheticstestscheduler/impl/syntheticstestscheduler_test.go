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

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl"
	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"
)

func Test_SyntheticsTestScheduler_StartAndStop(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("run_path", testDir)
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
	scheduler := newSyntheticsTestScheduler(configs, nil, l, nil, time.Now, &teststatsd.Client{}, nil, newStubPoller(t, l))
	assert.Nil(t, err)
	assert.False(t, scheduler.running)

	ctx := context.TODO()
	err = scheduler.start(ctx)
	assert.Nil(t, err)

	assert.True(t, scheduler.running)

	err = scheduler.start(ctx)
	assert.EqualError(t, err, "server already started")

	scheduler.stop()
	assert.False(t, scheduler.running)

	l.Close()
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

type tracerouteRunner struct {
	fn func(context.Context, config.Config) (payload.NetworkPath, error)
}

func (t *tracerouteRunner) Run(ctx context.Context, cfg config.Config) (payload.NetworkPath, error) {
	return t.fn(ctx, cfg)
}

func Test_SyntheticsTestScheduler_Processing(t *testing.T) {
	type testCase struct {
		name                  string
		testJSON              string
		expectedEventJSON     string
		expectedRunTraceroute func(context.Context, config.Config) (payload.NetworkPath, error)
	}

	testCases := []testCase{
		{
			name: "one test provided",
			testJSON: `{
					"version":1,"type":"network","subtype":"TCP",
					"config":{"assertions":[],"request":{"host":"example.com","port":443,"tcp_method":"SYN","probe_count":3,"traceroute_count":1,"max_ttl":30,"timeout":5,"source_service":"frontend","destination_service":"backend"}},
					"org_id":12345,"main_dc":"us1.staging.dog","public_id":"puf-9fm-c89","run_type":"scheduled"
				}`,
			expectedEventJSON: `{"location":{"id":"agent:test-hostname"},"_dd":{},"result":{"id":"4907739274636687553","initialId":"4907739274636687553","testFinishedAt":1756901488592,"testStartedAt":1756901488591,"testTriggeredAt":1756901488589,"assertions":[],"failure":null,"duration":1,"config":{"assertions":[],"request":{"destinationService":"backend","port":443,"maxTtl":30,"host":"example.com","tracerouteQueries":1,"e2eQueries":3,"sourceService":"frontend","timeout":5,"tcpMethod":"SYN"}},"netstats":{"packetsSent":0,"packetsReceived":0,"packetLossPercentage":0,"jitter":null,"latency":null,"hops":{"avg":0,"min":0,"max":0}},"netpath":{"timestamp":1756901488592,"agent_version":"","namespace":"","test_config_id":"puf-9fm-c89","test_result_id":"4907739274636687553","test_run_id":"test-run-id-111-example.com","origin":"synthetics","test_run_type":"scheduled","source_product":"synthetics","collector_type":"agent","protocol":"TCP","source":{"name":"test-hostname","display_name":"test-hostname","hostname":"test-hostname"},"destination":{"hostname":"example.com","port":443},"traceroute":{"runs":[{"run_id":"1","source":{"ip_address":"","port":0},"destination":{"ip_address":"","port":0},"hops":[{"ttl":0,"ip_address":"1.1.1.1","reachable":false},{"ttl":0,"ip_address":"1.1.1.2","reachable":false}]}],"hop_count":{"avg":0,"min":0,"max":0}},"e2e_probe":{"rtts":null,"packets_sent":0,"packets_received":0,"packet_loss_percentage":0,"jitter":0,"rtt":{"avg":0,"min":0,"max":0}}},"status":"passed","runType":"scheduled"},"test":{"id":"puf-9fm-c89","subType":"tcp","type":"network","version":1},"v":1}`,
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
			mockConfig.SetInTest("run_path", testDir)

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
			poller := newStubPoller(t, l)
			scheduler := newSyntheticsTestScheduler(configs, mockEpForwarder, l, &mockHostname{}, timeNowFn, &teststatsd.Client{}, &tracerouteRunner{tc.expectedRunTraceroute}, poller)
			assert.False(t, scheduler.running)

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

			var cfg common.SyntheticsTestConfig
			assert.Nil(t, json.Unmarshal([]byte(tc.testJSON), &cfg))
			poller.TestsChan <- SyntheticsTestCtx{nextRun: timeNowFn(), cfg: cfg}

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
		state:                        runningState{tests: map[string]*runningTestState{}},
		traceroute: &tracerouteRunner{fn: func(context.Context, config.Config) (payload.NetworkPath, error) {
			return payload.NetworkPath{
				TestRunID:   "path-123",
				Protocol:    payload.ProtocolTCP,
				Source:      payload.NetworkPathSource{Hostname: "src"},
				Destination: payload.NetworkPathDestination{Hostname: "dst", Port: 443},
			}, nil
		}},
		testPoller: newStubPoller(t, l),
	}

	gotCh := make(chan *workerResult, 1)
	scheduler.sendResult = func(w *workerResult) (string, error) {
		gotCh <- w
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
						Interval: 10,
					},
					nextRun: now.Add(-10 * time.Second),
				},
			},
		},
		flushInterval: 10 * time.Second,
		log:           l,
	}

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
	expectedNextRun := now
	assert.Equal(t, expectedNextRun, rt.nextRun)
}

func TestFlush_NoOpWhenPollerHealthy(t *testing.T) {
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
					cfg:     common.SyntheticsTestConfig{PublicID: "test1", Interval: 10},
					nextRun: now.Add(-10 * time.Second),
				},
			},
		},
		flushInterval: 10 * time.Second,
		log:           l,
		testPoller:    &testPoller{healthy: true},
	}

	scheduler.flush(context.Background(), now)

	select {
	case <-scheduler.syntheticsTestProcessingChan:
		t.Errorf("expected no enqueue while poller is healthy")
	case <-time.After(200 * time.Millisecond):
	}
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
						Interval: 10,
					},
					nextRun: now.Add(-10 * time.Second),
				},
				"test2": {
					cfg: common.SyntheticsTestConfig{
						PublicID: "test1",
						Interval: 10,
					},
					nextRun: now.Add(-10 * time.Second),
				},
			},
		},
		flushInterval: 100 * time.Millisecond,
		log:           tl,
	}

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
	expectedNextRun := now
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
