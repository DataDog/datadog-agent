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
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestGetGRPCStreamPort(t *testing.T) {
	t.Run("invalid port", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.SetWithoutSource("process_config.language_detection.grpc_port", "lorem ipsum")

		assert.Equal(t, config.DefaultProcessEntityStreamPort, getGRPCStreamPort(cfg))
	})

	t.Run("valid port", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.SetWithoutSource("process_config.language_detection.grpc_port", "1234")

		assert.Equal(t, 1234, getGRPCStreamPort(cfg))
	})

	t.Run("default", func(t *testing.T) {
		cfg := config.Mock(t)
		assert.Equal(t, config.DefaultProcessEntityStreamPort, getGRPCStreamPort(cfg))
	})
}

func TestStartStop(t *testing.T) {
	cfg := config.Mock(t)
	fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule()).Reset()

	extractor := NewWorkloadMetaExtractor(cfg)

	port := testutil.FreeTCPPort(t)
	cfg.SetWithoutSource("process_config.language_detection.grpc_port", port)
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
	fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule()).Reset()
	extractor := NewWorkloadMetaExtractor(cfg)

	port := testutil.FreeTCPPort(t)
	cfg.SetWithoutSource("process_config.language_detection.grpc_port", port)
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
	fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule()).Reset()
	extractor := NewWorkloadMetaExtractor(cfg)

	port := testutil.FreeTCPPort(t)
	cfg.SetWithoutSource("process_config.language_detection.grpc_port", port)
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

func TestStreamVersioning(t *testing.T) {
	extractor, _, conn, stream := setupGRPCTest(t)
	msg, err := stream.Recv()
	require.NoError(t, err)
	assert.EqualValues(t, 0, msg.EventID)

	extractor.diffChan <- &ProcessCacheDiff{cacheVersion: 1} // Simulate a cache update
	msg, err = stream.Recv()
	require.NoError(t, err)
	assert.EqualValues(t, 1, msg.EventID)

	extractor.diffChan <- &ProcessCacheDiff{cacheVersion: 3} // Simulate a missing message
	_, err = stream.Recv()
	assert.ErrorContains(t, err, "received version = 3; expected = 2")
	assert.Equal(t, conn.GetState(), connectivity.Ready) // Assert the underlying connection is still open

	// Make sure we are able to create a new stream using the same connection
	stream, err = pbgo.NewProcessEntityStreamClient(conn).StreamEntities(context.Background(), &pbgo.ProcessStreamEntitiesRequest{})
	require.NoError(t, err)
	msg, err = stream.Recv()
	require.NoError(t, err)
	assert.EqualValues(t, 0, msg.EventID)
}

func TestProcessEntityToEventSet(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		process *ProcessEntity
		event   *pbgo.ProcessEventSet
	}{
		{
			desc: "process with detected language",
			process: &ProcessEntity{
				Pid:          40,
				NsPid:        1,
				CreationTime: 5311456,
				Language: &languagemodels.Language{
					Name: languagemodels.Python,
				},
			},
			event: &pbgo.ProcessEventSet{
				Pid:          40,
				Nspid:        1,
				CreationTime: 5311456,
				Language:     &pbgo.Language{Name: "python"},
			},
		},
		{
			desc: "process without detected language",
			process: &ProcessEntity{
				Pid:          40,
				NsPid:        1,
				CreationTime: 5311456,
			},
			event: &pbgo.ProcessEventSet{
				Pid:          40,
				Nspid:        1,
				CreationTime: 5311456,
			},
		},
	} {
		event := processEntityToEventSet(tc.process)
		assert.Equal(t, tc.event, event)
	}
}

// TestSingleStream tests that there can only ever be a single stream at one time.
func TestSingleStream(t *testing.T) {
	ext, _, conn, originalStream := setupGRPCTest(t)
	_, err := originalStream.Recv() // fast-forward through the sync message
	require.NoError(t, err)

	newStream, err := pbgo.NewProcessEntityStreamClient(conn).StreamEntities(context.Background(), &pbgo.ProcessStreamEntitiesRequest{})
	require.NoError(t, err)

	_, err = newStream.Recv() // fast-forward through the sync message
	require.NoError(t, err)

	_, err = originalStream.Recv()
	assert.ErrorContains(t, err, DuplicateConnectionErr.Error())

	ext.diffChan <- &ProcessCacheDiff{cacheVersion: 1}
	_, err = newStream.Recv()
	assert.NoError(t, err)

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
	assert.Equal(t, expected.ContainerID, actual.ContainerID)
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

// setupGRPCTest a test extractor, server, and client connection.
// Cleanup is handled automatically via T.Cleanup().
func setupGRPCTest(t *testing.T) (*WorkloadMetaExtractor, *GRPCServer, *grpc.ClientConn, pbgo.ProcessEntityStream_StreamEntitiesClient) {
	t.Helper()

	cfg := config.Mock(t)
	port, err := testutil.FindTCPPort()
	require.NoError(t, err)
	cfg.SetWithoutSource("process_config.language_detection.grpc_port", port)
	fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule()).Reset()
	extractor := NewWorkloadMetaExtractor(cfg)

	grpcServer := NewGRPCServer(cfg, extractor)
	err = grpcServer.Start()
	require.NoError(t, err)
	t.Cleanup(grpcServer.Stop)

	cc, err := grpc.Dial(grpcServer.addr.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = cc.Close()
	})

	stream, err := pbgo.NewProcessEntityStreamClient(cc).StreamEntities(context.Background(), &pbgo.ProcessStreamEntitiesRequest{})
	require.NoError(t, err)

	return extractor, grpcServer, cc, stream
}
