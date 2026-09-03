// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/common"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/pathteststore"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
)

func TestAllowance(t *testing.T) {
	now := MockTimeNow()

	t.Run("basic is always in", func(t *testing.T) {
		a := newAllowance()
		for i := 0; i < standardAllowancePerHour+3; i++ {
			assert.True(t, a.inAllowance(payload.DynamicTestProfileBasic, now))
		}
	})

	t.Run("basic does not consume standard slots", func(t *testing.T) {
		a := newAllowance()
		for i := 0; i < 20; i++ {
			assert.True(t, a.inAllowance(payload.DynamicTestProfileBasic, now))
		}
		for i := 0; i < standardAllowancePerHour; i++ {
			assert.True(t, a.inAllowance(payload.DynamicTestProfileStandard, now), "standard %d", i)
		}
		assert.False(t, a.inAllowance(payload.DynamicTestProfileStandard, now))
		assert.True(t, a.inAllowance(payload.DynamicTestProfileBasic, now))
	})

	t.Run("unset profile is never in and does not consume", func(t *testing.T) {
		a := newAllowance()
		for i := 0; i < 20; i++ {
			assert.False(t, a.inAllowance("", now))
		}
		assert.True(t, a.inAllowance(payload.DynamicTestProfileStandard, now))
	})

	t.Run("standard first N then exhausted", func(t *testing.T) {
		a := newAllowance()
		for i := 0; i < standardAllowancePerHour; i++ {
			assert.True(t, a.inAllowance(payload.DynamicTestProfileStandard, now), "standard %d", i)
		}
		assert.False(t, a.inAllowance(payload.DynamicTestProfileStandard, now))
		assert.False(t, a.take(now))
	})

	t.Run("standard window resets at hour", func(t *testing.T) {
		a := newAllowance()
		for i := 0; i < standardAllowancePerHour; i++ {
			require.True(t, a.inAllowance(payload.DynamicTestProfileStandard, now))
		}
		assert.False(t, a.inAllowance(payload.DynamicTestProfileStandard, now.Add(standardAllowanceWindow-time.Nanosecond)))
		assert.True(t, a.inAllowance(payload.DynamicTestProfileStandard, now.Add(standardAllowanceWindow)))
	})

	t.Run("standard window resets after idle hours", func(t *testing.T) {
		a := newAllowance()
		require.True(t, a.inAllowance(payload.DynamicTestProfileStandard, now))
		assert.True(t, a.inAllowance(payload.DynamicTestProfileStandard, now.Add(2*standardAllowanceWindow)))
	})

	t.Run("time going backward stays in the same window", func(t *testing.T) {
		a := newAllowance()
		for i := 0; i < standardAllowancePerHour; i++ {
			require.True(t, a.inAllowance(payload.DynamicTestProfileStandard, now))
		}
		assert.False(t, a.inAllowance(payload.DynamicTestProfileStandard, now.Add(-time.Minute)))
	})

	t.Run("concurrent standard takes cap at N", func(t *testing.T) {
		a := newAllowance()
		results := make(chan bool, 20)
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- a.inAllowance(payload.DynamicTestProfileStandard, now)
			}()
		}
		wg.Wait()
		close(results)

		taken := 0
		for ok := range results {
			if ok {
				taken++
			}
		}
		assert.Equal(t, standardAllowancePerHour, taken)
	})
}

func TestRunTracerouteForPathAllowance(t *testing.T) {
	now := MockTimeNow()
	collector, emitted := newAllowanceCollector(t, succeedingAllowanceTraceroute())
	collector.TimeNowFn = func() time.Time { return now }

	for i := 0; i < standardAllowancePerHour+2; i++ {
		runAllowancePath(collector, payload.DynamicTestProfileStandard)
	}
	require.Len(t, *emitted, standardAllowancePerHour+2)
	for i, path := range *emitted {
		assert.Equal(t, payload.DynamicTestProfileStandard, path.DynamicTestProfile)
		if i < standardAllowancePerHour {
			assert.Equal(t, payload.DynamicTestClassCore, path.DynamicTestClass)
		} else {
			assert.Empty(t, path.DynamicTestClass)
		}
	}

	*emitted = nil
	runAllowancePath(collector, payload.DynamicTestProfileBasic)
	runAllowancePath(collector, "")
	runAllowanceNetflowPath(collector)
	runAllowancePath(collector, payload.DynamicTestProfileStandard)
	require.Len(t, *emitted, 4)
	assert.Equal(t, payload.DynamicTestClassCore, (*emitted)[0].DynamicTestClass)
	assert.Empty(t, (*emitted)[1].DynamicTestClass)
	assert.Empty(t, (*emitted)[2].DynamicTestClass)
	assert.Empty(t, (*emitted)[3].DynamicTestClass)

	now = now.Add(standardAllowanceWindow)
	*emitted = nil
	runAllowancePath(collector, payload.DynamicTestProfileStandard)
	require.Len(t, *emitted, 1)
	assert.Equal(t, payload.DynamicTestClassCore, (*emitted)[0].DynamicTestClass)
}

func TestRunTracerouteFailedDoesNotTakeAllowance(t *testing.T) {
	collector, emitted := newAllowanceCollector(t, &tracerouteRunner{func(_ context.Context, _ config.Config) (payload.NetworkPath, error) {
		return payload.NetworkPath{}, errors.New("boom")
	}})

	runAllowancePath(collector, payload.DynamicTestProfileStandard)
	assert.Empty(t, *emitted)
	assert.True(t, collector.allowance.take(MockTimeNow()))
}

func TestRunTracerouteInvalidPathDoesNotTakeAllowance(t *testing.T) {
	collector, emitted := newAllowanceCollector(t, &tracerouteRunner{func(_ context.Context, cfg config.Config) (payload.NetworkPath, error) {
		return payload.NetworkPath{
			Destination: payload.NetworkPathDestination{Hostname: cfg.DestHostname, Port: cfg.DestPort},
			Traceroute: payload.Traceroute{
				Runs: []payload.TracerouteRun{{
					Destination: payload.TracerouteDestination{IPAddress: net.IP{}, Port: cfg.DestPort},
				}},
			},
		}, nil
	}})

	runAllowancePath(collector, payload.DynamicTestProfileStandard)
	assert.Empty(t, *emitted)
	assert.True(t, collector.allowance.take(MockTimeNow()))
}

func newAllowanceCollector(t *testing.T, traceroute *tracerouteRunner) (*npCollectorImpl, *[]payload.NetworkPath) {
	t.Helper()
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.enabled": true,
		"network_path.collector.filters":              []map[string]any{},
	}, &teststatsd.Client{}, traceroute)

	var emitted []payload.NetworkPath
	mockEpForwarder := eventplatformimpl.NewMockEventPlatformForwarder(gomock.NewController(t))
	collector.epForwarder = mockEpForwarder
	mockEpForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), eventplatform.EventTypeNetworkPath).
		DoAndReturn(func(msg *message.Message, _ string) error {
			var path payload.NetworkPath
			require.NoError(t, json.Unmarshal(msg.GetContent(), &path))
			emitted = append(emitted, path)
			return nil
		}).AnyTimes()
	return collector, &emitted
}

func succeedingAllowanceTraceroute() *tracerouteRunner {
	return &tracerouteRunner{func(_ context.Context, cfg config.Config) (payload.NetworkPath, error) {
		return payload.NetworkPath{
			Protocol: cfg.Protocol,
			Destination: payload.NetworkPathDestination{
				Hostname: cfg.DestHostname,
				Port:     cfg.DestPort,
			},
			Traceroute: payload.Traceroute{
				Runs: []payload.TracerouteRun{{
					RunID: "aa-bb-cc",
					Destination: payload.TracerouteDestination{
						IPAddress: net.ParseIP("10.0.0.2"),
						Port:      cfg.DestPort,
					},
				}},
			},
		}, nil
	}}
}

func runAllowancePath(collector *npCollectorImpl, profile payload.DynamicTestProfile) {
	collector.runTracerouteForPath(&pathteststore.PathtestContext{
		Pathtest: &common.Pathtest{
			Hostname:           "10.0.0.2",
			Port:               443,
			Protocol:           payload.ProtocolTCP,
			Origin:             payload.PathOriginNetworkTraffic,
			DynamicTestProfile: profile,
		},
	})
}

func runAllowanceNetflowPath(collector *npCollectorImpl) {
	collector.runTracerouteForPath(&pathteststore.PathtestContext{
		Pathtest: &common.Pathtest{
			Hostname: "10.0.0.2",
			Port:     443,
			Protocol: payload.ProtocolTCP,
			Origin:   payload.PathOriginNetflow,
		},
	})
}
