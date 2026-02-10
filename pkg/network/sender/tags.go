// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"maps"
	"strconv"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/twmb/murmur3"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/encoding/marshal"
	"github.com/DataDog/datadog-agent/pkg/network/indexedset"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/tls"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func formatTags(c network.ConnectionStats, tagsSet *indexedset.IndexedSet[string], connDynamicTags map[string]struct{}) ([]int32, uint32) {
	var checksum uint32

	staticTags := tls.GetStaticTags(c.StaticTags)
	tagsIdx := make([]int32, 0, len(staticTags)+len(connDynamicTags)+len(c.Tags))

	for _, tag := range staticTags {
		checksum ^= murmur3.StringSum32(tag)
		tagsIdx = append(tagsIdx, tagsSet.Add(tag))
	}

	// Dynamic tags
	for tag := range connDynamicTags {
		checksum ^= murmur3.StringSum32(tag)
		tagsIdx = append(tagsIdx, tagsSet.Add(tag))
	}

	// other tags, e.g., from process env vars like DD_ENV, etc.
	for _, tag := range c.Tags {
		if tag == nil {
			continue
		}
		t, ok := tag.Get().(string)
		if !ok {
			continue
		}
		checksum ^= murmur3.StringSum32(t)
		tagsIdx = append(tagsIdx, tagsSet.Add(t))
	}

	return tagsIdx, checksum
}

func (d *directSender) addContainerTags(builder *model.ConnectionBuilder, sourceContainerID *intern.Value, tagsEncoder model.TagEncoder) {
	if sourceContainerID != nil {
		if d.resolver.shouldTagContainer(sourceContainerID) {
			if cid := getInternedString(sourceContainerID); cid != "" {
				if entityTags, err := d.tagger.Tag(types.NewEntityID(types.ContainerID, cid), types.HighCardinality); err != nil {
					if log.ShouldLog(log.DebugLvl) {
						log.Debugf("error getting tags for container %s: %v", cid, err)
					}
				} else if len(entityTags) > 0 {
					builder.SetLocalContainerTagsIndex(int32(tagsEncoder.Encode(entityTags)))
					return
				}
			}
		}
	}
	builder.SetLocalContainerTagsIndex(-1)
}

func (d *directSender) addTags(builder *model.ConnectionBuilder, nc network.ConnectionStats, tagsSet *indexedset.IndexedSet[string], usmEncoders []marshal.USMEncoder, connectionsTagsEncoder model.TagEncoder) {
	var staticTags uint64
	dynamicTags := nc.TLSTags.GetDynamicTags()
	for _, encoder := range usmEncoders {
		encoderStaticTags, encoderDynamicTags := encoder.EncodeConnection(nc, builder)
		staticTags |= encoderStaticTags
		maps.Copy(dynamicTags, encoderDynamicTags)
	}

	nc.StaticTags |= staticTags
	tagIndexes, tagChecksum := formatTags(nc, tagsSet, dynamicTags)
	builder.SetTagsChecksum(tagChecksum)

	tagsStr := tagsSet.Subset(tagIndexes)
	if nc.Pid > 0 {
		var serviceTags []string
		if dsc := directSenderConsumerInstance.Load(); dsc != nil {
			serviceTags = dsc.extractor.GetServiceContext(int32(nc.Pid))
		}
		tagsStr = append(tagsStr, serviceTags...)
		processEntityID := types.NewEntityID(types.Process, strconv.Itoa(int(nc.Pid)))
		if processTags, err := d.tagger.Tag(processEntityID, types.HighCardinality); err != nil {
			log.Debugf("error getting tags for process %v: %v", nc.Pid, err)
		} else {
			tagsStr = append(tagsStr, processTags...)
		}
	}
	if len(tagsStr) > 0 {
		builder.SetTagsIdx(int32(connectionsTagsEncoder.Encode(tagsStr)))
	} else {
		builder.SetTagsIdx(-1)
	}
}
