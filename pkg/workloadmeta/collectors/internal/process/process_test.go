package process

import (
	"context"
	"fmt"
	"testing"
	"time"

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
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const testCid = "containersAreAwesome"

type collectorTest struct {
	probe       *mocks.Probe
	collectChan chan<- time.Time
	collector   *collector
	store       *workloadmeta.MockStore
	stream      pbgo.ProcessEntityStream_StreamEntitiesClient
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

func setUpCollectorTest(t *testing.T) *collectorTest {
	t.Helper()

	cfg := config.Mock(t)
	port, err := testutil.FindTCPPort()
	require.NoError(t, err)
	cfg.Set("process_config.language_detection.grpc_port", port)

	wlmExtractor := workloadmetaExtractor.NewWorkloadMetaExtractor(cfg)
	grpcServer := workloadmetaExtractor.NewGRPCServer(cfg, wlmExtractor)

	mockProcessData, probe := checks.NewProcessDataWithMockProbe(t)
	mockProcessData.Register(wlmExtractor)

	oldCollector := c
	t.Cleanup(func() {
		c = oldCollector
	})

	collectChan := make(chan time.Time)

	store := workloadmeta.NewMockStore()

	c := &collector{
		ddConfig:         cfg,
		processData:      mockProcessData,
		wlmExtractor:     wlmExtractor,
		grpcServer:       grpcServer,
		pidToCid:         make(map[int]string),
		collectionTicker: collectChan,
	}
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, c.Start(ctx, store))
	t.Cleanup(cancel)

	return &collectorTest{
		collector:   c,
		probe:       probe,
		collectChan: collectChan,
		store:       store,
		stream:      acquireStream(t, port),
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

func TestProcessCollector(t *testing.T) {
	c := setUpCollectorTest(t)
	c.setupProcs()

	// Fast-forward through sync message
	resp, err := c.stream.Recv()
	require.NoError(t, err)

	c.collectChan <- time.Now()

	resp, err = c.stream.Recv()
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
	c.collectChan <- time.Now()

	resp, err = c.stream.Recv()
	assert.NoError(t, err)

	require.Len(t, resp.SetEvents, 1)
	evt = resp.SetEvents[0]
	assert.EqualValues(t, 1, evt.Pid)
	assert.EqualValues(t, 1, evt.CreationTime)
	assert.Equal(t, testCid, evt.ContainerId)
}

// Assert that the collector is only Enabled if the process check is disabled and
// the remote process collector is Enabled.
func TestEnabled(t *testing.T) {
	t.Run("process check Enabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_collection.Enabled", true)

		enabled, err := Enabled(cfg)
		assert.False(t, enabled)
		assert.Error(t, err)
	})

	t.Run("remote collector disabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_collection.Enabled", false)
		cfg.Set("workloadmeta.remote_process_collector.Enabled", false)

		enabled, err := Enabled(cfg)
		assert.False(t, enabled)
		assert.Error(t, err)
	})

	t.Run("Enabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_collection.Enabled", false)
		cfg.Set("workloadmeta.remote_process_collector.Enabled", true)

		enabled, err := Enabled(cfg)
		assert.True(t, enabled)
		assert.NoError(t, err)
	})
}
