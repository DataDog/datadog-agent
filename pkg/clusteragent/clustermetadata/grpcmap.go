// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func answerToProto(answer cm.LookupAnswer) *pb.ClusterMetadataAnswer {
	kind := pb.ClusterMetadataAnswerKind_CLUSTER_METADATA_ABSENT
	switch answer.Kind {
	case cm.AnswerFound:
		kind = pb.ClusterMetadataAnswerKind_CLUSTER_METADATA_FOUND
	case cm.AnswerNotMine:
		kind = pb.ClusterMetadataAnswerKind_CLUSTER_METADATA_NOT_MINE
	case cm.AnswerNotReady:
		kind = pb.ClusterMetadataAnswerKind_CLUSTER_METADATA_NOT_READY
	}
	return &pb.ClusterMetadataAnswer{Kind: kind, Tags: answer.Tags}
}

func answerFromProto(answer *pb.ClusterMetadataAnswer) cm.LookupAnswer {
	kind := cm.AnswerAbsent
	switch answer.GetKind() {
	case pb.ClusterMetadataAnswerKind_CLUSTER_METADATA_FOUND:
		kind = cm.AnswerFound
	case pb.ClusterMetadataAnswerKind_CLUSTER_METADATA_NOT_MINE:
		kind = cm.AnswerNotMine
	case pb.ClusterMetadataAnswerKind_CLUSTER_METADATA_NOT_READY:
		kind = cm.AnswerNotReady
	}
	return cm.LookupAnswer{Kind: kind, Tags: answer.GetTags()}
}

func scopeFromProto(scope *pb.ClusterMetadataScope) cm.Scope {
	return cm.Scope{
		Consumer:    scope.GetConsumer(),
		Cardinality: cardinalityFromProto(scope.GetCardinality()),
	}
}

func scopeToProto(scope cm.Scope) *pb.ClusterMetadataScope {
	return &pb.ClusterMetadataScope{
		Consumer:    scope.Consumer,
		Cardinality: cardinalityToProto(scope.Cardinality),
	}
}

// snapshotToProto converts a shard snapshot into its wire form.
func snapshotToProto(snapshot cm.ShardSnapshot) *pb.ClusterMetadataSnapshot {
	response := &pb.ClusterMetadataSnapshot{Nodes: snapshot.Nodes}
	for _, event := range snapshot.Events {
		response.Events = append(response.Events, eventToProto(event))
	}
	return response
}

// ringToProto converts a ring view into its wire form.
func ringToProto(ring cm.RingInfo) *pb.ClusterMetadataRing {
	response := &pb.ClusterMetadataRing{}
	for _, member := range ring.Members {
		response.Members = append(response.Members, &pb.ClusterMetadataRingMember{
			Name:  member.Name,
			Nodes: member.Nodes,
			Ready: member.Ready,
		})
	}
	return response
}

// eventToProto converts one stream event into its wire form.
func eventToProto(event cm.NodeEvent) *pb.ClusterMetadataNodeEvent {
	return &pb.ClusterMetadataNodeEvent{
		Kind:      event.Kind,
		Namespace: event.Namespace,
		Name:      event.Name,
		Deleted:   event.Deleted,
		Tags:      event.Tags,
	}
}

// snapshotFromProto converts a wire snapshot back into the local form.
func snapshotFromProto(snapshot *pb.ClusterMetadataSnapshot) cm.ShardSnapshot {
	result := cm.ShardSnapshot{Nodes: snapshot.GetNodes()}
	for _, event := range snapshot.GetEvents() {
		result.Events = append(result.Events, cm.NodeEvent{
			Kind:      event.GetKind(),
			Namespace: event.GetNamespace(),
			Name:      event.GetName(),
			Deleted:   event.GetDeleted(),
			Tags:      event.GetTags(),
		})
	}
	return result
}

// ringFromProto converts a wire ring view back into the local form.
func ringFromProto(ring *pb.ClusterMetadataRing) cm.RingInfo {
	info := cm.RingInfo{}
	for _, member := range ring.GetMembers() {
		info.Members = append(info.Members, cm.RingMember{
			Name:  member.GetName(),
			Nodes: member.GetNodes(),
			Ready: member.GetReady(),
		})
	}
	return info
}

func cardinalityFromProto(c pb.ClusterMetadataCardinality) taggertypes.TagCardinality {
	switch c {
	case pb.ClusterMetadataCardinality_CLUSTER_METADATA_ORCHESTRATOR:
		return taggertypes.OrchestratorCardinality
	case pb.ClusterMetadataCardinality_CLUSTER_METADATA_HIGH:
		return taggertypes.HighCardinality
	default:
		return taggertypes.LowCardinality
	}
}

func cardinalityToProto(c taggertypes.TagCardinality) pb.ClusterMetadataCardinality {
	switch c {
	case taggertypes.OrchestratorCardinality:
		return pb.ClusterMetadataCardinality_CLUSTER_METADATA_ORCHESTRATOR
	case taggertypes.HighCardinality:
		return pb.ClusterMetadataCardinality_CLUSTER_METADATA_HIGH
	default:
		return pb.ClusterMetadataCardinality_CLUSTER_METADATA_LOW
	}
}
