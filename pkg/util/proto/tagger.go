// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package proto contains protobuf related helpers.
package proto

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
)

// Tagger2PbEntityID helper to convert an Entity ID to its expected protobuf format.
func Tagger2PbEntityID(entityID string) (*pb.EntityId, error) {
	panic("not called")
}

// Tagger2PbEntityEvent helper to convert a native EntityEvent type to its protobuf representation.
func Tagger2PbEntityEvent(event types.EntityEvent) (*pb.StreamTagsEvent, error) {
	panic("not called")
}

// Pb2TaggerEntityID helper to convert a protobuf Entity ID to its expected format.
func Pb2TaggerEntityID(entityID *pb.EntityId) (string, error) {
	panic("not called")
}

// Pb2TaggerCardinality helper to convert protobuf cardinality to native tag cardinality.
func Pb2TaggerCardinality(pbCardinality pb.TagCardinality) (collectors.TagCardinality, error) {
	panic("not called")
}
