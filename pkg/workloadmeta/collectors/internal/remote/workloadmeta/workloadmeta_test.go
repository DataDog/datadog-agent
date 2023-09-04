// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

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
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/proto"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/remote"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type serverSecure struct {
	pbgo.UnimplementedAgentSecureServer
	workloadmetaServer *server.Server
}

func (*serverSecure) TaggerStreamEntities(*pbgo.StreamTagsRequest, pbgo.AgentSecure_TaggerStreamEntitiesServer) error {
	return status.Errorf(codes.Unimplemented, "method TaggerStreamEntities not implemented")
}
func (*serverSecure) TaggerFetchEntity(context.Context, *pbgo.FetchEntityRequest) (*pbgo.FetchEntityResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method TaggerFetchEntity not implemented")
}
func (*serverSecure) DogstatsdCaptureTrigger(context.Context, *pbgo.CaptureTriggerRequest) (*pbgo.CaptureTriggerResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DogstatsdCaptureTrigger not implemented")
}
func (*serverSecure) DogstatsdSetTaggerState(context.Context, *pbgo.TaggerState) (*pbgo.TaggerStateResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DogstatsdSetTaggerState not implemented")
}
func (*serverSecure) ClientGetConfigs(context.Context, *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ClientGetConfigs not implemented")
}
func (*serverSecure) GetConfigState(context.Context, *emptypb.Empty) (*pbgo.GetStateConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetConfigState not implemented")
}
func (s *serverSecure) WorkloadmetaStreamEntities(in *pbgo.WorkloadmetaStreamRequest, out pbgo.AgentSecure_WorkloadmetaStreamEntitiesServer) error {
	return s.workloadmetaServer.StreamEntities(in, out)
}

func TestHandleWorkloadmetaStreamResponse(t *testing.T) {
	protoWorkloadmetaEvent := &pbgo.WorkloadmetaEvent{
		Type: pbgo.WorkloadmetaEventType_EVENT_TYPE_SET,
		Container: &pbgo.Container{
			EntityId: &pbgo.WorkloadmetaEntityId{
				Kind: pbgo.WorkloadmetaKind_CONTAINER,
				Id:   "123",
			},
			EntityMeta: &pbgo.EntityMeta{
				Name:      "abc",
				Namespace: "default",
				Annotations: map[string]string{
					"an_annotation": "an_annotation_value",
				},
				Labels: map[string]string{
					"a_label": "a_label_value",
				},
			},
			EnvVars: map[string]string{
				"an_env": "an_env_val",
			},
			Hostname: "test_host",
			Image: &pbgo.ContainerImage{
				Id:        "123",
				RawName:   "datadog/agent:7",
				Name:      "datadog/agent",
				ShortName: "agent",
				Tag:       "7",
			},
			NetworkIps: map[string]string{
				"net1": "10.0.0.1",
				"net2": "192.168.0.1",
			},
			Pid: 0,
			Ports: []*pbgo.ContainerPort{
				{
					Port:     2000,
					Protocol: "tcp",
				},
			},
			Runtime: pbgo.Runtime_CONTAINERD,
			State: &pbgo.ContainerState{
				Running:    true,
				Status:     pbgo.ContainerStatus_CONTAINER_STATUS_RUNNING,
				Health:     pbgo.ContainerHealth_CONTAINER_HEALTH_HEALTHY,
				CreatedAt:  time.Time{}.Unix(),
				StartedAt:  time.Time{}.Unix(),
				FinishedAt: time.Time{}.Unix(),
				ExitCode:   0,
			},
			CollectorTags: []string{
				"tag1",
			},
		},
	}

	expectedEvent, err := proto.WorkloadmetaEventFromProtoEvent(protoWorkloadmetaEvent)
	require.NoError(t, err)

	mockResponse := &pbgo.WorkloadmetaStreamResponse{
		Events: []*pbgo.WorkloadmetaEvent{protoWorkloadmetaEvent},
	}

	streamhandler := &streamHandler{}
	collectorEvents, err := streamhandler.HandleResponse(mockResponse)

	require.NoError(t, err)
	assert.Len(t, collectorEvents, 1)
	assert.Equal(t, collectorEvents[0], workloadmeta.CollectorEvent{
		Type:   expectedEvent.Type,
		Entity: expectedEvent.Entity,
		Source: workloadmeta.SourceRemoteWorkloadmeta,
	})
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

	// workloadmeta server
	mockServerStore := workloadmeta.NewMockStore()
	server := &serverSecure{workloadmetaServer: server.NewServer(mockServerStore)}

	// gRPC server
	grpcServer := grpc.NewServer()
	pbgo.RegisterAgentSecureServer(grpcServer, server)

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

	// gRPC client
	collector := &remote.GenericCollector{
		StreamHandler: &streamHandler{
			port: port,
		},
		Insecure: true,
	}

	mockClientStore := workloadmeta.NewMockStore()
	ctx, cancel := context.WithCancel(context.TODO())
	err = collector.Start(ctx, mockClientStore)
	require.NoError(t, err)

	// Start straming entites
	expectedContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "ctr-id",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "ctr-name",
		},
		Image: workloadmeta.ContainerImage{
			Name: "ctr-image",
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
		State: workloadmeta.ContainerState{
			Running:    true,
			Status:     workloadmeta.ContainerStatusRunning,
			Health:     workloadmeta.ContainerHealthHealthy,
			CreatedAt:  time.Time{},
			StartedAt:  time.Time{},
			FinishedAt: time.Time{},
		},
		EnvVars: map[string]string{
			"DD_SERVICE":  "my-svc",
			"DD_ENV":      "prod",
			"DD_VERSION":  "v1",
			"NOT_ALLOWED": "not-allowed",
		},
	}

	mockServerStore.Notify(
		[]workloadmeta.CollectorEvent{
			{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceAll,
				Entity: expectedContainer,
			},
		},
	)

	time.Sleep(2 * time.Second)
	cancel()

	container, err := mockClientStore.GetContainer("ctr-id")
	require.NoError(t, err)
	assert.Equal(t, container, expectedContainer)
}
