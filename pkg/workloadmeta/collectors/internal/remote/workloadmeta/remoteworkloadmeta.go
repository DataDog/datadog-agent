// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteworkloadmeta

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	protoutils "github.com/DataDog/datadog-agent/pkg/util/proto"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/remote"
)

const (
	collectorID = "remote-workloadmeta"
)

type client struct {
	cl pb.AgentSecureClient
}

type stream struct {
	cl pbgo.AgentSecure_WorkloadmetaStreamEntitiesClient
}

func (s *stream) Recv() (interface{}, error) {
	return s.cl.Recv()
}

func (c *client) StreamEntities(ctx context.Context, opts ...grpc.CallOption) (remote.Stream, error) {
	streamcl, err := c.cl.WorkloadmetaStreamEntities(
		ctx,
		&pb.WorkloadmetaStreamRequest{
			Filter: nil, // Subscribes to all events
		},
	)
	if err != nil {
		return nil, err
	}
	return &stream{cl: streamcl}, nil
}

func init() {
	grpclog.SetLoggerV2(grpcutil.NewLogger())

	workloadmeta.RegisterRemoteCollector(collectorID, func() workloadmeta.Collector {
		return &remote.GenericCollector{
			NewClient:       NewAgentSecureClient,
			ResponseHandler: handleWorkloadmetaStreamResponse,
			Port:            config.Datadog.GetInt("cmd_port"),
		}
	})
}

func NewAgentSecureClient(cc grpc.ClientConnInterface) remote.RemoteGrpcClient {
	return &client{cl: pb.NewAgentSecureClient(cc)}
}

func handleWorkloadmetaStreamResponse(resp interface{}) ([]workloadmeta.CollectorEvent, error) {
	response, ok := resp.(*pb.WorkloadmetaStreamResponse)
	if !ok {
		return nil, fmt.Errorf("incorrect response type")
	}
	var collectorEvents []workloadmeta.CollectorEvent

	for _, protoEvent := range response.Events {
		workloadmetaEvent, err := protoutils.WorkloadmetaEventFromProtoEvent(protoEvent)
		if err != nil {
			return nil, err
		}

		collectorEvent := workloadmeta.CollectorEvent{
			Type:   workloadmetaEvent.Type,
			Source: workloadmeta.SourceRemoteWorkloadmeta,
			Entity: workloadmetaEvent.Entity,
		}

		collectorEvents = append(collectorEvents, collectorEvent)
	}

	return collectorEvents, nil
}
