// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package processcollector

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/remote"
	"google.golang.org/grpc"
)

const dummySubscriber = "dummy-subscriber"

type mockServer struct {
	pbgo.UnimplementedProcessEntityStreamServer

	responses     []*pbgo.ProcessStreamResponse
	errorResponse bool // first response is an error
}

// StreamEntities sends the responses back to the client
func (s *mockServer) StreamEntities(_ *pbgo.ProcessStreamEntitiesRequest, out pbgo.ProcessEntityStream_StreamEntitiesServer) error {
	// Handle error response for the first request
	if s.errorResponse {
		s.errorResponse = false // Reset error response for subsequent requests
		return xerrors.New("dummy first error")
	}

	for _, response := range s.responses {
		err := out.Send(response)
		if err != nil {
			panic(err)
		}
	}

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
		preEvents []workloadmeta.CollectorEvent

		serverResponses   []*pbgo.ProcessStreamResponse
		expectedProcesses []*workloadmeta.Process
		errorResponse     bool
	}{
		{
			name: "initially empty",
			serverResponses: []*pbgo.ProcessStreamResponse{
				{
					EventID: 0,
					SetEvents: []*pbgo.ProcessEventSet{
						{
							Pid:          123,
							Nspid:        345,
							ContainerId:  "cid",
							Language:     &pbgo.Language{Name: string(languagemodels.Java)},
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
					ContainerID:  "cid",
					Language:     &languagemodels.Language{Name: languagemodels.Java},
					CreationTime: time.UnixMilli(creationTime),
				},
			},
		},
		{
			name: "two response with set",
			serverResponses: []*pbgo.ProcessStreamResponse{
				{
					EventID: 0,
					SetEvents: []*pbgo.ProcessEventSet{
						{
							Pid:          123,
							Nspid:        345,
							ContainerId:  "cid",
							Language:     &pbgo.Language{Name: string(languagemodels.Java)},
							CreationTime: creationTime,
						},
					},
				},
				{
					EventID: 1,
					SetEvents: []*pbgo.ProcessEventSet{
						{
							Pid:          345,
							Nspid:        567,
							ContainerId:  "cid",
							Language:     &pbgo.Language{Name: string(languagemodels.Java)},
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
					ContainerID:  "cid",
					Language:     &languagemodels.Language{Name: languagemodels.Java},
					CreationTime: time.UnixMilli(creationTime),
				},
				{
					EntityID: workloadmeta.EntityID{
						ID:   "345",
						Kind: workloadmeta.KindProcess,
					},
					NsPid:        567,
					ContainerID:  "cid",
					Language:     &languagemodels.Language{Name: languagemodels.Java},
					CreationTime: time.UnixMilli(creationTime),
				},
			},
		},
		{
			name: "one set one unset",
			serverResponses: []*pbgo.ProcessStreamResponse{
				{
					EventID: 0,
					SetEvents: []*pbgo.ProcessEventSet{
						{
							Pid:          123,
							Nspid:        345,
							ContainerId:  "cid",
							Language:     &pbgo.Language{Name: string(languagemodels.Java)},
							CreationTime: creationTime,
						},
					},
				},
				{
					EventID: 1,
					UnsetEvents: []*pbgo.ProcessEventUnset{
						{
							Pid: 123,
						},
					},
				},
			},
			expectedProcesses: []*workloadmeta.Process{},
		},
		{
			name: "on resync",
			preEvents: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRemoteProcessCollector,
					Entity: &workloadmeta.Process{
						EntityID: workloadmeta.EntityID{
							ID:   "123",
							Kind: workloadmeta.KindProcess,
						},
						NsPid:        345,
						ContainerID:  "cid",
						Language:     &languagemodels.Language{Name: languagemodels.Java},
						CreationTime: time.UnixMilli(creationTime),
					},
				},
			},
			serverResponses: []*pbgo.ProcessStreamResponse{
				{
					EventID: 0,
					SetEvents: []*pbgo.ProcessEventSet{
						{
							Pid:          345,
							Nspid:        678,
							ContainerId:  "cid",
							Language:     &pbgo.Language{Name: string(languagemodels.Java)},
							CreationTime: creationTime,
						},
					},
				},
			},
			expectedProcesses: []*workloadmeta.Process{
				{
					EntityID: workloadmeta.EntityID{
						ID:   "345",
						Kind: workloadmeta.KindProcess,
					},
					NsPid:        678,
					ContainerID:  "cid",
					Language:     &languagemodels.Language{Name: languagemodels.Java},
					CreationTime: time.UnixMilli(creationTime),
				},
			},
			errorResponse: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockConfig := config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
			mockConfig.Set("language_detection.enabled", true)
			// remote process collector server (process agent)
			server := &mockServer{
				responses:     test.serverResponses,
				errorResponse: test.errorResponse,
			}
			grpcServer := grpc.NewServer()
			pbgo.RegisterProcessEntityStreamServer(grpcServer, server)

			lis, err := net.Listen("tcp", ":0")
			require.NoError(t, err)

			go func() {
				err := grpcServer.Serve(lis)
				require.NoError(t, err)
			}()
			defer grpcServer.Stop()

			_, portStr, err := net.SplitHostPort(lis.Addr().String())
			require.NoError(t, err)
			port, err := strconv.Atoi(portStr)
			require.NoError(t, err)

			// gRPC client (core agent)
			collector := &remote.GenericCollector{
				StreamHandler: &streamHandler{
					Config: mockConfig,
					port:   port,
				},
				Insecure: true,
			}

			mockStore := workloadmeta.NewMockStore()
			mockStore.Notify(test.preEvents)

			ctx, cancel := context.WithCancel(context.TODO())

			// Subscribe to the mockStore
			ch := mockStore.Subscribe(dummySubscriber, workloadmeta.NormalPriority, nil)

			// Collect process data
			err = collector.Start(ctx, mockStore)
			require.NoError(t, err)

			// Number of events expected. Each response can hold multiple events, either Set or Unset
			numberOfEvents := len(test.preEvents)
			for _, ev := range test.serverResponses {
				numberOfEvents += len(ev.SetEvents) + len(ev.UnsetEvents)
			}

			// Keep listening to workloadmeta until enough events are received. It is possible that the
			// first bundle does not hold any events. Thus, it is required to look at the number of events
			// in the bundle.
			for i := 0; i < numberOfEvents; {
				bundle := <-ch
				close(bundle.Ch)
				i += len(bundle.Events)
			}
			mockStore.Unsubscribe(ch)
			cancel()

			// Verify final state
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
