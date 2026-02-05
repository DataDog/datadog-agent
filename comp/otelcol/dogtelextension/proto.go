// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogtelextension

import (
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// pb2TaggerCardinality converts protobuf cardinality to native tag cardinality
func pb2TaggerCardinality(pbCardinality pb.TagCardinality) (types.TagCardinality, error) {
	switch pbCardinality {
	case pb.TagCardinality_LOW:
		return types.LowCardinality, nil
	case pb.TagCardinality_ORCHESTRATOR:
		return types.OrchestratorCardinality, nil
	case pb.TagCardinality_HIGH:
		return types.HighCardinality, nil
	}

	return 0, status.Errorf(codes.InvalidArgument, "invalid cardinality %q", pbCardinality)
}

// tagger2PbEntityEvent converts a native EntityEvent type to its protobuf representation
func tagger2PbEntityEvent(event types.EntityEvent) (*pb.StreamTagsEvent, error) {
	entity := event.Entity
	entityID := &pb.EntityId{
		Prefix: string(entity.ID.GetPrefix()),
		Uid:    entity.ID.GetID(),
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
