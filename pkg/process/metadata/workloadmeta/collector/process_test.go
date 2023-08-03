// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	workloadmetaExtractor "github.com/DataDog/datadog-agent/pkg/process/metadata/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const testCid = "containersAreAwesome"

type collectorTest struct {
	probe     *mocks.Probe
	clock     *clock.Mock
	collector *Collector
	store     *workloadmeta.MockStore
	stream    pbgo.ProcessEntityStream_StreamEntitiesClient
}

func acquireStream(t *testing.T, port int) pbgo.ProcessEntityStream_StreamEntitiesClient {
	t.Helper()

	cc, err := grpc.Dial(fmt.Sprintf("localhost:%v", port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = cc.Close()
	})

	stream, err := pbgo.NewProcessEntityStreamClient(cc).StreamEntities(context.Background(), &pbgo.ProcessStreamEntitiesRequest{})
	require.NoError(t, err)

	return stream
}

func setFlavor(t *testing.T, newFlavor string) {
	t.Helper()

	oldFlavor := flavor.GetFlavor()
	flavor.SetFlavor(newFlavor)
	t.Cleanup(func() { flavor.SetFlavor(oldFlavor) })
}

func setUpCollectorTest(t *testing.T) *collectorTest {
	t.Helper()

	setFlavor(t, flavor.ProcessAgent)

	cfg := config.Mock(t)
	port, err := testutil.FindTCPPort()
	require.NoError(t, err)
	cfg.Set("process_config.language_detection.grpc_port", port)
	cfg.Set("workloadmeta.remote_process_collector.enabled", true)

	wlmExtractor := workloadmetaExtractor.NewWorkloadMetaExtractor(cfg)
	grpcServer := workloadmetaExtractor.NewGRPCServer(cfg, wlmExtractor)

	mockProcessData, probe := checks.NewProcessDataWithMockProbe(t)
	mockProcessData.Register(wlmExtractor)

	store := workloadmeta.NewMockStore()

	mockClock := clock.NewMock()

	c := &Collector{
		ddConfig:        cfg,
		processData:     mockProcessData,
		wlmExtractor:    wlmExtractor,
		grpcServer:      grpcServer,
		pidToCid:        make(map[int]string),
		collectionClock: mockClock,
	}
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, c.Start(ctx, store))
	t.Cleanup(cancel)

	return &collectorTest{
		collector: c,
		probe:     probe,
		clock:     mockClock,
		store:     store,
		stream:    acquireStream(t, port),
	}
}

func (c *collectorTest) setupProcs() {
	c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(map[int32]*procutil.Process{
		1: {
			Pid:     1,
			Cmdline: []string{"some-awesome-game", "--with-rgb", "--give-me-more-fps"},
			Stats:   &procutil.Stats{CreateTime: 1},
		},
	}, nil).Maybe()
}

func (c *collectorTest) waitForContainerUpdate(t *testing.T, cont *workloadmeta.Container) {
	t.Helper()

	c.store.SetEntity(cont)
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		assert.Contains(t, c.collector.pidToCid, cont.PID)
	}, 5*time.Second, 1*time.Second)
}

// Tick sets up the collector to collect processes by advancing the clock
func (c *collectorTest) tick() {
	c.clock.Add(config.DefaultLocalProcessCollectorInterval)
}

func TestProcessCollector(t *testing.T) {
	c := setUpCollectorTest(t)
	c.setupProcs()

	// Fast-forward through sync message
	_, err := c.stream.Recv()
	require.NoError(t, err)

	c.tick()
	resp, err := c.stream.Recv()
	assert.NoError(t, err)

	require.Len(t, resp.SetEvents, 1)
	evt := resp.SetEvents[0]
	assert.EqualValues(t, 1, evt.Pid)
	assert.EqualValues(t, 1, evt.CreationTime)

	// Now test that this process updates with container id when the store is changed
	c.waitForContainerUpdate(t, &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   testCid,
		},
		PID: 1,
	})

	c.tick()
	resp, err = c.stream.Recv()
	assert.NoError(t, err)

	require.Len(t, resp.SetEvents, 1)
	evt = resp.SetEvents[0]
	assert.EqualValues(t, 1, evt.Pid)
	assert.EqualValues(t, 1, evt.CreationTime)
	assert.Equal(t, testCid, evt.ContainerId)
}

// Assert that the collector is only enabled if the process check is disabled and
// the remote process collector is enabled.
func TestEnabled(t *testing.T) {
	type testCase struct {
		name                                                    string
		processCollectionEnabled, remoteProcessCollectorEnabled bool
		expectEnabled                                           bool
		flavor                                                  string
	}

	testCases := []testCase{
		{
			name:                          "process check enabled",
			processCollectionEnabled:      true,
			remoteProcessCollectorEnabled: false,
			flavor:                        flavor.ProcessAgent,
			expectEnabled:                 false,
		},
		{
			name:                          "remote collector disabled",
			processCollectionEnabled:      false,
			remoteProcessCollectorEnabled: false,
			flavor:                        flavor.ProcessAgent,
			expectEnabled:                 false,
		},
		{
			name:                          "collector enabled",
			processCollectionEnabled:      false,
			remoteProcessCollectorEnabled: true,
			flavor:                        flavor.ProcessAgent,
			expectEnabled:                 true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setFlavor(t, tc.flavor)

			cfg := config.Mock(t)
			cfg.Set("process_config.process_collection.enabled", tc.processCollectionEnabled)
			cfg.Set("workloadmeta.remote_process_collector.Enabled", tc.remoteProcessCollectorEnabled)

			assert.Equal(t, tc.expectEnabled, Enabled(cfg))
		})
	}
}
