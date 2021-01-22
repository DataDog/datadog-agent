// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/status"

	pb "github.com/DataDog/datadog-agent/cmd/agent/api/pb"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	hostutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	taggerStreamSendTimeout = 1 * time.Minute
)

type server struct {
	pb.UnimplementedAgentServer
}

type serverSecure struct {
	pb.UnimplementedAgentServer
}

func (s *server) GetHostname(ctx context.Context, in *pb.HostnameRequest) (*pb.HostnameReply, error) {
	h, err := hostutil.GetHostname()
	if err != nil {
		return &pb.HostnameReply{}, err
	}
	return &pb.HostnameReply{Hostname: h}, nil
}

// AuthFuncOverride implements the `grpc_auth.ServiceAuthFuncOverride` interface which allows
// override of the AuthFunc registered with the unary interceptor.
//
// see: https://godoc.org/github.com/grpc-ecosystem/go-grpc-middleware/auth#ServiceAuthFuncOverride
func (s *server) AuthFuncOverride(ctx context.Context, fullMethodName string) (context.Context, error) {
	return ctx, nil
}

func (s *serverSecure) DogstatsdCaptureTrigger(ctx context.Context, req *pb.CaptureTriggerRequest) (*pb.CaptureTriggerResponse, error) {
	d, err := time.ParseDuration(req.GetDuration())
	if err != nil {
		return &pb.CaptureTriggerResponse{}, err
	}

	err = common.DSD.Capture(d)
	if err != nil {
		return &pb.CaptureTriggerResponse{}, err
	}

	// wait for the capture to start
	for !common.DSD.TCapture.IsOngoing() {
		time.Sleep(500 * time.Millisecond)
	}

	p, err := common.DSD.TCapture.Path()
	if err != nil {
		return &pb.CaptureTriggerResponse{}, err
	}

	return &pb.CaptureTriggerResponse{Path: p}, nil
}

// StreamTags subscribes to added, removed, or changed entities in the Tagger
// and streams them to clients as pb.StreamTagsResponse events. Filtering is as
// of yet not implemented.
func (s *serverSecure) TaggerStreamEntities(in *pb.StreamTagsRequest, out pb.AgentSecure_TaggerStreamEntitiesServer) error {
	cardinality, err := pb2taggerCardinality(in.Cardinality)
	if err != nil {
		return err
	}

	// NOTE: StreamTagsRequest can specify filters, but they cannot be
	// implemented since the tagger has no concept of container metadata.
	// these filters will be introduced when we implement a container
	// metadata service that can receive them as is from the tagger.

	t := tagger.GetDefaultTagger()
	eventCh := t.Subscribe(cardinality)
	defer t.Unsubscribe(eventCh)

	for events := range eventCh {
		responseEvents := make([]*pb.StreamTagsEvent, 0, len(events))
		for _, event := range events {
			e, err := tagger2pbEntityEvent(event)
			if err != nil {
				log.Warnf("can't convert tagger entity to protobuf: %s", err)
				continue
			}

			responseEvents = append(responseEvents, e)
		}

		err = grpc.DoWithTimeout(func() error {
			return out.Send(&pb.StreamTagsResponse{
				Events: responseEvents,
			})
		}, taggerStreamSendTimeout)
		if err != nil {
			log.Warnf("error sending tagger event: %s", err)
			telemetry.ServerStreamErrors.Inc()
			return err
		}
	}

	return nil
}

// FetchEntity fetches an entity from the Tagger with the desired cardinality tags.
func (s *serverSecure) TaggerFetchEntity(ctx context.Context, in *pb.FetchEntityRequest) (*pb.FetchEntityResponse, error) {
	if in.Id == nil {
		return nil, status.Errorf(codes.InvalidArgument, `missing "id" parameter`)
	}

	entityID := fmt.Sprintf("%s://%s", in.Id.Prefix, in.Id.Uid)
	cardinality, err := pb2taggerCardinality(in.Cardinality)
	if err != nil {
		return nil, err
	}

	tags, err := tagger.Tag(entityID, cardinality)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err)
	}

	return &pb.FetchEntityResponse{
		Id:          in.Id,
		Cardinality: in.Cardinality,
		Tags:        tags,
	}, nil
}

func tagger2pbEntityID(entityID string) (*pb.EntityId, error) {
	parts := strings.SplitN(entityID, "://", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid entity id %q", entityID)
	}

	return &pb.EntityId{
		Prefix: parts[0],
		Uid:    parts[1],
	}, nil
}

func tagger2pbEntityEvent(event types.EntityEvent) (*pb.StreamTagsEvent, error) {
	entity := event.Entity
	entityID, err := tagger2pbEntityID(entity.ID)
	if err != nil {
		return nil, err
	}

	var eventType pb.EventType
	switch event.EventType {
	case types.EventTypeAdded:
		eventType = pb.EventType_ADDED
	case types.EventTypeModified:
		eventType = pb.EventType_MODIFIED
	case types.EventTypeDeleted:
		eventType = pb.EventType_DELETED
	default:
		return nil, fmt.Errorf("invalid event type %q", event.EventType)
	}

	return &pb.StreamTagsEvent{
		Type: eventType,
		Entity: &pb.Entity{
			Id:                          entityID,
			HighCardinalityTags:         entity.HighCardinalityTags,
			OrchestratorCardinalityTags: entity.OrchestratorCardinalityTags,
			LowCardinalityTags:          entity.LowCardinalityTags,
			StandardTags:                entity.StandardTags,
		},
	}, nil
}

func pb2taggerCardinality(pbCardinality pb.TagCardinality) (collectors.TagCardinality, error) {
	switch pbCardinality {
	case pb.TagCardinality_LOW:
		return collectors.LowCardinality, nil
	case pb.TagCardinality_ORCHESTRATOR:
		return collectors.OrchestratorCardinality, nil
	case pb.TagCardinality_HIGH:
		return collectors.HighCardinality, nil
	}

	return 0, status.Errorf(codes.InvalidArgument, "invalid cardinality %q", pbCardinality)
}

func init() {
	grpclog.SetLoggerV2(grpc.NewLogger())
}
