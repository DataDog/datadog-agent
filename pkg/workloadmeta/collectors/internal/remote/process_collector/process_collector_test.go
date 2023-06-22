// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processcollector

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/proto"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/remote"
	"google.golang.org/grpc"
)

type mockServer struct {
	pbgo.UnimplementedProcessEntityStreamServer
	eventID     int32
	setEvents   []*pb.ProcessEventSet
	unsetEvents []*pb.ProcessEventUnset
	timeout     time.Duration
	answered    bool
}

func (s *mockServer) StreamEntities(_ *pbgo.ProcessStreamEntitiesRequest, out pbgo.ProcessEntityStream_StreamEntitiesServer) error {
	if s.answered {
		return nil
	}
	err := ddgrpc.DoWithTimeout(func() error {
		return out.Send(&pb.ProcessStreamResponse{
			EventID:     s.eventID,
			SetEvents:   s.setEvents,
			UnsetEvents: s.unsetEvents,
		})
	}, s.timeout)
	if err != nil {
		panic(err)
	}
	s.answered = true
	return nil
}

func TestHandleProcessStreamResponse(t *testing.T) {
	unsetEvents := []*pbgo.ProcessEventUnset{
		{
			Pid: 456,
		},
		{
			Pid: 789,
		},
	}

	setEvents := []*pbgo.ProcessEventSet{
		{
			Pid: 123,
		},
		{
			Pid: 345,
		},
	}
	expectedEvents := make([]workloadmeta.CollectorEvent, 0, len(unsetEvents)+len(setEvents))
	for i := range unsetEvents {
		expectedUnsetEvent, err := proto.WorkloadmetaEventFromProcessEventUnset(unsetEvents[i])
		require.NoError(t, err)
		expectedEvents = append(expectedEvents, workloadmeta.CollectorEvent{
			Type:   expectedUnsetEvent.Type,
			Source: workloadmeta.SourceRemoteProcessCollector,
			Entity: expectedUnsetEvent.Entity,
		})
	}

	for i := range setEvents {
		expectedSetEvent, err := proto.WorkloadmetaEventFromProcessEventSet(setEvents[i])
		require.NoError(t, err)
		expectedEvents = append(expectedEvents, workloadmeta.CollectorEvent{
			Type:   expectedSetEvent.Type,
			Source: workloadmeta.SourceRemoteProcessCollector,
			Entity: expectedSetEvent.Entity,
		})
	}

	mockResponse := &pbgo.ProcessStreamResponse{
		EventID:     0,
		SetEvents:   setEvents,
		UnsetEvents: unsetEvents,
	}

	streamhandler := &remoteProcessCollectorStreamHandler{}
	collectorEvents, err := streamhandler.HandleResponse(mockResponse)
	require.NoError(t, err)

	assert.Equal(t, expectedEvents, collectorEvents)
}

func TestCollection(t *testing.T) {
	// Create Auth Token for the client
	if _, err := os.Stat(security.GetAuthTokenFilepath()); os.IsNotExist(err) {
		security.CreateOrFetchToken()
		defer func() {
			// cleanup
			os.Remove(security.GetAuthTokenFilepath())
		}()
	}
	creationTime := time.Now().Unix()
	tests := []struct {
		name              string
		eventID           int32
		preEvents         []workloadmeta.CollectorEvent
		setEvents         []*pb.ProcessEventSet
		unsetEvents       []*pb.ProcessEventUnset
		timeout           time.Duration
		expectedProcesses []*workloadmeta.Process
	}{
		{
			name:    "initially empty",
			eventID: 0,
			setEvents: []*pb.ProcessEventSet{
				{
					Pid:          123,
					Nspid:        345,
					ContainerId:  "cid",
					Language:     &pb.Language{Name: string(languagemodels.Java)},
					CreationTime: creationTime,
				},
			},
			timeout: 1 * time.Second,
			expectedProcesses: []*workloadmeta.Process{
				{
					EntityID: workloadmeta.EntityID{
						ID:   "123",
						Kind: workloadmeta.KindProcess,
					},
					NsPid:        345,
					ContainerId:  "cid",
					Language:     &languagemodels.Language{Name: languagemodels.Java},
					CreationTime: time.Unix(creationTime, 0),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// remote process collector server (process agent)
			server := &mockServer{
				eventID:     test.eventID,
				setEvents:   test.setEvents,
				unsetEvents: test.unsetEvents,
				timeout:     test.timeout,
			}
			grpcServer := grpc.NewServer()
			pbgo.RegisterProcessEntityStreamServer(grpcServer, server)

			lis, err := net.Listen("tcp", ":0")
			require.NoError(t, err)

			go func() {
				err := grpcServer.Serve(lis)
				require.NoError(t, err)
			}()

			_, portStr, err := net.SplitHostPort(lis.Addr().String())
			require.NoError(t, err)
			port, err := strconv.Atoi(portStr)
			require.NoError(t, err)

			// gRPC client (core agent)
			collector := &remote.GenericCollector{
				StreamHandler: &remoteProcessCollectorStreamHandler{},
				Port:          port,
				Insecure:      true,
			}

			mockStore := workloadmeta.NewMockStore()
			mockStore.Notify(test.preEvents)
			err = collector.Collect(context.TODO(), mockStore)
			require.NoError(t, err)

			time.Sleep(2 * time.Second)

			for i := range test.expectedProcesses {
				pid, err := strconv.Atoi(test.expectedProcesses[i].ID)
				require.NoError(t, err)
				p, err := mockStore.GetProcess(int32(pid))
				assert.NoError(t, err)
				assert.Equal(t, test.expectedProcesses[i], p)
			}
		})
	}
}
