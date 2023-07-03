// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
)

func TestGetGRPCStreamPort(t *testing.T) {
	t.Run("invalid port", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.language_detection.grpc_port", "lorem ipsum")

		assert.Equal(t, config.DefaultProcessEntityStreamPort, getGRPCStreamPort(cfg))
	})

	t.Run("valid port", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.language_detection.grpc_port", "1234")

		assert.Equal(t, 1234, getGRPCStreamPort(cfg))
	})

	t.Run("default", func(t *testing.T) {
		cfg := config.Mock(t)
		assert.Equal(t, config.DefaultProcessEntityStreamPort, getGRPCStreamPort(cfg))
	})
}

func TestStartStop(t *testing.T) {
	cfg := config.Mock(t)

	extractor := NewWorkloadMetaExtractor(cfg)
	cfg.Set("process_config.language_detection.grpc_port", "0") // Tell the os to choose a port for us to reduce flakiness
	srv := NewGRPCServer(config.Mock(t), extractor)

	err := srv.Start()
	assert.NoError(t, err)

	stopped := make(chan struct{})
	go func() {
		srv.Stop()
		stopped <- struct{}{}
	}()
	assert.Eventually(t, func() bool {
		select {
		case <-stopped:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

func TestStreamServer(t *testing.T) {
	var (
		proc1 = testProc(Pid1, []string{"java", "mydatabase.jar"})
		proc2 = testProc(Pid2, []string{"python", "myprogram.py"})
		proc3 = testProc(Pid3, []string{"corrina", "--at-her-best"})
	)

	cfg := config.Mock(t)
	extractor := NewWorkloadMetaExtractor(cfg)

	cfg.Set("process_config.language_detection.grpc_port", "0") // Tell the os to choose a port for us to reduce flakiness
	srv := NewGRPCServer(cfg, extractor)
	require.NoError(t, srv.Start())
	require.NotNil(t, srv.addr)
	defer srv.Stop()

	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1,
		Pid2: proc2,
	})
	// Drop first cache diff before gRPC connection is created
	<-extractor.ProcessCacheDiff()

	cc, err := grpc.Dial(srv.addr.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()
	streamClient := pbgo.NewProcessEntityStreamClient(cc)

	// Test that the sync message is sent to the client
	stream, err := streamClient.StreamEntities(context.Background(), &pbgo.ProcessStreamEntitiesRequest{})
	require.NoError(t, err)
	defer stream.CloseSend()

	msg, err := stream.Recv()
	sort.SliceStable(msg.SetEvents, func(i, j int) bool {
		return msg.SetEvents[i].Pid < msg.SetEvents[j].Pid
	})
	require.NoError(t, err)
	assertEqualStreamEntitiesResponse(t, &pbgo.ProcessStreamResponse{
		EventID: 1,
		SetEvents: []*pbgo.ProcessEventSet{
			toEventSet(proc1),
			toEventSet(proc2),
		},
	}, msg)

	// Test that diffs are sent to the client
	// proc1 and proc2 terminated
	extractor.Extract(map[int32]*procutil.Process{
		Pid3: proc3,
	})

	msg, err = stream.Recv()
	require.NoError(t, err)
	assertEqualStreamEntitiesResponse(t, &pbgo.ProcessStreamResponse{
		EventID: 2,
		SetEvents: []*pbgo.ProcessEventSet{
			toEventSet(proc3),
		},
		UnsetEvents: []*pbgo.ProcessEventUnset{
			toEventUnset(proc1),
			toEventUnset(proc2),
		},
	}, msg)

	// proc3 terminated
	extractor.Extract(map[int32]*procutil.Process{})
	msg, err = stream.Recv()
	require.NoError(t, err)
	assertEqualStreamEntitiesResponse(t, &pbgo.ProcessStreamResponse{
		EventID:   3,
		SetEvents: []*pbgo.ProcessEventSet{},
		UnsetEvents: []*pbgo.ProcessEventUnset{
			toEventUnset(proc3),
		},
	}, msg)
}

func TestStreamServerDropRedundantCacheDiff(t *testing.T) {
	var (
		proc1 = testProc(Pid1, []string{"java", "mydatabase.jar"})
		proc2 = testProc(Pid2, []string{"python", "myprogram.py"})
		proc3 = testProc(Pid3, []string{"corrina", "--at-her-best"})
	)

	cfg := config.Mock(t)
	extractor := NewWorkloadMetaExtractor(cfg)

	cfg.Set("process_config.language_detection.grpc_port", "0") // Tell the os to choose a port for us to reduce flakiness
	srv := NewGRPCServer(cfg, extractor)
	require.NoError(t, srv.Start())
	require.NotNil(t, srv.addr)
	defer srv.Stop()

	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1,
		Pid2: proc2,
	})

	cc, err := grpc.Dial(srv.addr.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()
	streamClient := pbgo.NewProcessEntityStreamClient(cc)

	// Test that the sync message is sent to the client
	stream, err := streamClient.StreamEntities(context.Background(), &pbgo.ProcessStreamEntitiesRequest{})
	require.NoError(t, err)
	defer stream.CloseSend()

	msg, err := stream.Recv()
	sort.SliceStable(msg.SetEvents, func(i, j int) bool {
		return msg.SetEvents[i].Pid < msg.SetEvents[j].Pid
	})
	require.NoError(t, err)
	assertEqualStreamEntitiesResponse(t, &pbgo.ProcessStreamResponse{
		EventID: 1,
		SetEvents: []*pbgo.ProcessEventSet{
			toEventSet(proc1),
			toEventSet(proc2),
		},
	}, msg)

	// Verify that first diff is not sent to the client since its version is equal to the first cache snapshot
	// sent on the connection creation
	// proc1 and proc2 terminated
	extractor.Extract(map[int32]*procutil.Process{
		Pid3: proc3,
	})

	msg, err = stream.Recv()
	require.NoError(t, err)
	assertEqualStreamEntitiesResponse(t, &pbgo.ProcessStreamResponse{
		EventID: 2,
		SetEvents: []*pbgo.ProcessEventSet{
			toEventSet(proc3),
		},
		UnsetEvents: []*pbgo.ProcessEventUnset{
			toEventUnset(proc1),
			toEventUnset(proc2),
		},
	}, msg)
}

func assertEqualStreamEntitiesResponse(t *testing.T, expected, actual *pbgo.ProcessStreamResponse) {
	t.Helper()

	sort.SliceStable(actual.SetEvents, func(i, j int) bool {
		return actual.SetEvents[i].Pid < actual.SetEvents[j].Pid
	})
	sort.SliceStable(actual.UnsetEvents, func(i, j int) bool {
		return actual.UnsetEvents[i].Pid < actual.UnsetEvents[j].Pid
	})

	assert.Equal(t, expected.EventID, actual.EventID)

	assert.Len(t, expected.SetEvents, len(actual.SetEvents))
	assert.Len(t, expected.UnsetEvents, len(actual.UnsetEvents))

	for i, expectedSet := range expected.SetEvents {
		actualSet := expected.SetEvents[i]
		assertSetEvent(t, expectedSet, actualSet)
	}
	for i, expectedUnset := range expected.UnsetEvents {
		actualSet := expected.UnsetEvents[i]
		assertUnsetEvent(t, expectedUnset, actualSet)
	}
}

func assertSetEvent(t *testing.T, expected, actual *pbgo.ProcessEventSet) {
	t.Helper()

	assert.Equal(t, expected.Pid, actual.Pid)
	assert.Equal(t, expected.Nspid, actual.Nspid)
	assert.Equal(t, expected.ContainerId, actual.ContainerId)
	assert.Equal(t, expected.CreationTime, actual.CreationTime)
	if expected.Language != nil {
		assert.Equal(t, expected.Language.Name, actual.Language.Name)
	}
}

func assertUnsetEvent(t *testing.T, expected, actual *pbgo.ProcessEventUnset) {
	t.Helper()

	assert.Equal(t, expected.Pid, actual.Pid)
}

func toEventSet(proc *procutil.Process) *pbgo.ProcessEventSet {
	return &pbgo.ProcessEventSet{Pid: proc.Pid}
}

func toEventUnset(proc *procutil.Process) *pbgo.ProcessEventUnset {
	return &pbgo.ProcessEventUnset{Pid: proc.Pid}
}
