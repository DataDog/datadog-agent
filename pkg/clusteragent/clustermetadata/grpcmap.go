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
