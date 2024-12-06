// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package processcollector implements the remote process collector for
// Workloadmeta.
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
	"go.uber.org/fx"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const dummySubscriber = "dummy-subscriber"

func newMockServer(ctx context.Context, responses []*pbgo.ProcessStreamResponse, errorResponse bool) *mockServer {
	ctx, cancelFunc := context.WithCancel(ctx)
	return &mockServer{
		ctx:           ctx,
		cancelFunc:    cancelFunc,
		responses:     responses,
		errorResponse: errorResponse,
	}
}

type mockServer struct {
	pbgo.UnimplementedProcessEntityStreamServer
	ctx           context.Context
	cancelFunc    context.CancelFunc
	responses     []*pbgo.ProcessStreamResponse
	errorResponse bool // first response is an error
}

func (s *mockServer) stop() {
	s.cancelFunc()
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
			return err
		}
	}

	<-s.ctx.Done()
	return nil
}

func TestCollection(t *testing.T) {
	// Create Auth Token for the client
	if _, err := os.Stat(security.GetAuthTokenFilepath(pkgconfigsetup.Datadog())); os.IsNotExist(err) {
		security.CreateOrFetchToken(pkgconfigsetup.Datadog())
		defer func() {
			// cleanup
			os.Remove(security.GetAuthTokenFilepath(pkgconfigsetup.Datadog()))
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
							ContainerID:  "cid",
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
							ContainerID:  "cid",
							Language:     &pbgo.Language{Name: string(languagemodels.Java)},
							CreationTime: creationTime,
						},
					},
				},
				{
					EventID: 1,
					SetEvents: []*pbgo.ProcessEventSet{
						{
							Pid:          321,
							Nspid:        765,
							ContainerID:  "cid",
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
						ID:   "321",
						Kind: workloadmeta.KindProcess,
					},
					NsPid:        765,
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
							ContainerID:  "cid",
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
							ContainerID:  "cid",
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

			overrides := map[string]interface{}{
				"language_detection.enabled": true,
			}

			// We do not inject any collectors here; we instantiate
			// and initialize it out-of-band below. That's OK.
			mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				fx.Replace(config.MockParams{Overrides: overrides}),
				workloadmetafxmock.MockModule(workloadmeta.Params{AgentType: workloadmeta.Remote}),
			))

			time.Sleep(time.Second)

			ctx := context.Background()

			// remote process collector server (process agent)
			server := newMockServer(ctx, test.serverResponses, test.errorResponse)
			defer server.stop()

			grpcServer := grpc.NewServer()
			pbgo.RegisterProcessEntityStreamServer(grpcServer, server)

			lis, err := net.Listen("tcp", "127.0.0.1:0")
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
				StreamHandler: &streamHandler{
					Reader: mockStore.GetConfig(),
					port:   port,
				},
				Insecure: true,
			}

			mockStore.Notify(test.preEvents)

			// Subscribe to the mockStore
			ch := mockStore.Subscribe(dummySubscriber, workloadmeta.NormalPriority, nil)

			collectorCtx, cancelCollectorCtxFunc := context.WithCancel(ctx)
			// Collect process data
			err = collector.Start(collectorCtx, mockStore)
			require.NoError(t, err)

			// Number of events expected. Each response can hold multiple events, either Set or Unset
			expectedNumberOfEvents := len(test.preEvents)
			for _, ev := range test.serverResponses {
				expectedNumberOfEvents += len(ev.SetEvents) + len(ev.UnsetEvents)
			}

			// Keep listening to workloadmeta until enough events are received. It is possible that the
			// first bundle does not hold any events. Thus, it is required to look at the number of events
			// in the bundle.
			// Also, when a problem occurs and a re-sync is triggered, we might
			// receive duplicate events, so we need to keep a map of received
			// events to account for duplicates.
			eventsReceived := make(map[workloadmeta.Event]struct{})
			for len(eventsReceived) < expectedNumberOfEvents {
				bundle := <-ch
				bundle.Acknowledge()

				for _, ev := range bundle.Events {
					eventsReceived[ev] = struct{}{}
				}
			}

			mockStore.Unsubscribe(ch)
			grpcServer.Stop()
			cancelCollectorCtxFunc()

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
