// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package proto contains protobuf related helpers.
package proto

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// Tagger2PbEntityID helper to convert an Entity ID to its expected protobuf format.
func Tagger2PbEntityID(entityID string) (*pb.EntityId, error) {
	parts := strings.SplitN(entityID, "://", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid entity id %q", entityID)
	}

	return &pb.EntityId{
		Prefix: parts[0],
		Uid:    parts[1],
	}, nil
}

// Tagger2PbEntityEvent helper to convert a native EntityEvent type to its protobuf representation.
func Tagger2PbEntityEvent(event types.EntityEvent) (*pb.StreamTagsEvent, error) {
	entity := event.Entity
	entityID, err := Tagger2PbEntityID(entity.ID)
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

// Pb2TaggerEntityID helper to convert a protobuf Entity ID to its expected format.
func Pb2TaggerEntityID(entityID *pb.EntityId) (string, error) {
	if entityID == nil {
		return "", errors.New("Invalid entityID argument")
	}

	return fmt.Sprintf("%s://%s", entityID.Prefix, entityID.Uid), nil
}

// Pb2TaggerCardinality helper to convert protobuf cardinality to native tag cardinality.
func Pb2TaggerCardinality(pbCardinality pb.TagCardinality) (collectors.TagCardinality, error) {
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
