// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadmeta implements the remote workloadmeta Collector.
package workloadmeta

import (
	"context"
	"fmt"

	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/proto"
	"github.com/DataDog/datadog-agent/pkg/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

const (
	collectorID = "remote-workloadmeta"
)

// Params defines the parameters of the remote workloadmeta collector.
type Params struct {
	Filter *workloadmeta.Filter
}

type dependencies struct {
	fx.In

	Params Params
}

type client struct {
	cl     pb.AgentSecureClient
	filter *workloadmeta.Filter
}

func (c *client) StreamEntities(ctx context.Context, _ ...grpc.CallOption) (remote.Stream, error) {
	protoFilter, err := proto.ProtobufFilterFromWorkloadmetaFilter(c.filter)
	if err != nil {
		return nil, err
	}

	streamcl, err := c.cl.WorkloadmetaStreamEntities(
		ctx,
		&pb.WorkloadmetaStreamRequest{
			Filter: protoFilter,
		},
	)
	if err != nil {
		return nil, err
	}
	return &stream{cl: streamcl}, nil
}

type stream struct {
	cl pb.AgentSecure_WorkloadmetaStreamEntitiesClient
}

func (s *stream) Recv() (interface{}, error) {
	return s.cl.Recv()
}

type streamHandler struct {
	port   int
	filter *workloadmeta.Filter
	config.Config
}

// NewCollector returns a CollectorProvider to build a remote workloadmeta collector, and an error if any.
func NewCollector(deps dependencies) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &remote.GenericCollector{
			CollectorID: collectorID,
			StreamHandler: &streamHandler{
				filter: deps.Params.Filter,
				Config: config.Datadog,
			},
			Catalog: workloadmeta.Remote,
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func init() {
	// TODO(components): verify the grpclogin is initialized elsewhere and cleanup
	grpclog.SetLoggerV2(grpcutil.NewLogger())
}

func (s *streamHandler) Port() int {
	if s.port == 0 {
		return s.Config.GetInt("cmd_port")
	}
	// for tests
	return s.port
}

func (s *streamHandler) NewClient(cc grpc.ClientConnInterface) remote.GrpcClient {
	return &client{
		cl:     pb.NewAgentSecureClient(cc),
		filter: s.filter,
	}
}

// IsEnabled always return true for the remote workloadmeta because it uses the remote catalog
func (s *streamHandler) IsEnabled() bool {
	return true
}

func (s *streamHandler) HandleResponse(resp interface{}) ([]workloadmeta.CollectorEvent, error) {
	response, ok := resp.(*pb.WorkloadmetaStreamResponse)
	if !ok {
		return nil, fmt.Errorf("incorrect response type")
	}
	var collectorEvents []workloadmeta.CollectorEvent

	for _, protoEvent := range response.Events {
		workloadmetaEvent, err := proto.WorkloadmetaEventFromProtoEvent(protoEvent)
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

func (s *streamHandler) HandleResync(store workloadmeta.Component, events []workloadmeta.CollectorEvent) {
	entities := make([]workloadmeta.Entity, 0, len(events))
	for _, event := range events {
		entities = append(entities, event.Entity)
	}
	// This should be the first response that we got from workloadmeta after
	// we lost the connection and specified that a re-sync is needed. So, at
	// this point we know that "entities" contains all the existing entities
	// in the store, because when a client subscribes to workloadmeta, the
	// first response is always a bundle of events with all the existing
	// entities in the store that match the filters specified (see
	// workloadmeta.Store#Subscribe).
	store.Reset(entities, workloadmeta.SourceRemoteWorkloadmeta)
}
