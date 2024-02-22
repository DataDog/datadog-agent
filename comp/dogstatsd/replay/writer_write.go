// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build !serverless

package replay

import (
	"encoding/binary"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	protoutils "github.com/DataDog/datadog-agent/pkg/util/proto"
	proto "github.com/golang/protobuf/proto"
)

// writeState writes the tagger state to the capture file.
func (tc *TrafficCaptureWriter) writeState() (int, error) {

	pbState := &pb.TaggerState{
		State:  make(map[string]*pb.Entity),
		PidMap: tc.taggerState,
	}

	// iterate entities
	for _, id := range tc.taggerState {
		entity, err := tagger.GetEntity(id)
		if err != nil {
			log.Warnf("There was no entity for container id: %v present in the tagger", entity)
			continue
		}

		entityID, err := protoutils.Tagger2PbEntityID(entity.ID)
		if err != nil {
			log.Warnf("unable to compute valid EntityID for %v", id)
			continue
		}

		entry := pb.Entity{
			// TODO: Hash:               entity.Hash,
			Id:                          entityID,
			HighCardinalityTags:         entity.HighCardinalityTags,
			OrchestratorCardinalityTags: entity.OrchestratorCardinalityTags,
			LowCardinalityTags:          entity.LowCardinalityTags,
			StandardTags:                entity.StandardTags,
		}
		pbState.State[id] = &entry
	}

	log.Debugf("Going to write STATE: %#v", pbState)

	s, err := proto.Marshal(pbState)
	if err != nil {
		return 0, err
	}

	// Record State Separator
	if n, err := tc.writer.Write([]byte{0, 0, 0, 0}); err != nil {
		return n, err
	}

	// Record State
	n, err := tc.writer.Write(s)

	// Record size
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(len(s)))

	if n, err := tc.writer.Write(buf); err != nil {
		return n, err
	}

	// n + 4 bytes for separator + 4 bytes for state size
	return n + 8, err
}
