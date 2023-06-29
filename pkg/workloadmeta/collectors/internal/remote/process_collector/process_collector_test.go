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
	"golang.org/x/xerrors"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/remote"
	"google.golang.org/grpc"
)

type mockServer struct {
	pbgo.UnimplementedProcessEntityStreamServer

	responses     []*pb.ProcessStreamResponse
	errorResponse bool // first response is an error

	currentResponse int
}

func (s *mockServer) StreamEntities(req *pbgo.ProcessStreamEntitiesRequest, out pbgo.ProcessEntityStream_StreamEntitiesServer) error {
	// Handle error response for the first request
	if s.errorResponse {
		s.errorResponse = false // Reset error response for subsequent requests
		return xerrors.New("dummy first error")
	}

	if s.currentResponse >= len(s.responses) {
		return nil
	}

	err := out.Send(s.responses[s.currentResponse])
	if err != nil {
		panic(err)
	}
	s.currentResponse++

	return nil
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
		name      string
		eventID   int32
		preEvents []workloadmeta.CollectorEvent

		serverResponses   []*pb.ProcessStreamResponse
		expectedProcesses []*workloadmeta.Process
		errorResponse     bool
	}{
		{
			name:    "initially empty",
			eventID: 0,
			serverResponses: []*pb.ProcessStreamResponse{
				{
					EventID: 0,
					SetEvents: []*pb.ProcessEventSet{
						{
							Pid:          123,
							Nspid:        345,
							ContainerId:  "cid",
							Language:     &pb.Language{Name: string(languagemodels.Java)},
							CreationTime: creationTime,
						},
					},
				},
			},

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
		{
			name:    "two response with set",
			eventID: 0,
			serverResponses: []*pb.ProcessStreamResponse{
				{
					EventID: 0,
					SetEvents: []*pb.ProcessEventSet{
						{
							Pid:          123,
							Nspid:        345,
							ContainerId:  "cid",
							Language:     &pb.Language{Name: string(languagemodels.Java)},
							CreationTime: creationTime,
						},
					},
				},
				{
					EventID: 1,
					SetEvents: []*pb.ProcessEventSet{
						{
							Pid:          345,
							Nspid:        567,
							ContainerId:  "cid",
							Language:     &pb.Language{Name: string(languagemodels.Java)},
							CreationTime: creationTime,
						},
					},
				},
			},

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
				{
					EntityID: workloadmeta.EntityID{
						ID:   "345",
						Kind: workloadmeta.KindProcess,
					},
					NsPid:        567,
					ContainerId:  "cid",
					Language:     &languagemodels.Language{Name: languagemodels.Java},
					CreationTime: time.Unix(creationTime, 0),
				},
			},
		},
		{
			name:    "one set one unset",
			eventID: 0,
			serverResponses: []*pb.ProcessStreamResponse{
				{
					EventID: 0,
					SetEvents: []*pb.ProcessEventSet{
						{
							Pid:          123,
							Nspid:        345,
							ContainerId:  "cid",
							Language:     &pb.Language{Name: string(languagemodels.Java)},
							CreationTime: creationTime,
						},
					},
				},
				{
					EventID: 1,
					UnsetEvents: []*pb.ProcessEventUnset{
						{
							Pid: 123,
						},
					},
				},
			},
			expectedProcesses: []*workloadmeta.Process{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// remote process collector server (process agent)
			server := &mockServer{
				responses:       test.serverResponses,
				errorResponse:   test.errorResponse,
				currentResponse: 0,
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

			time.Sleep(1 * time.Second)

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
