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

// getContainersForExplicitTagging returns all containers that are relevant for explicit tagging based on the current connections.
// A container is relevant for explicit tagging if it appears as a local container in the given connections, and
// it started less than `expected_tags_duration` ago, or the agent start time is within the `expected_tags_duration` window.
func (d *directSender) getContainersForExplicitTagging(currentConnections *network.Connections) map[string]types.EntityID {
	// Get a list of all container IDs that are currently belong with the given connections.
	ids := make(map[string]struct{})
	for _, conn := range currentConnections.Conns {
		if conn.ContainerID.Source != nil {
			ids[conn.ContainerID.Source.Get().(string)] = struct{}{}
		}
	}

	currentTime := time.Now()
	duration := d.sysprobeconfig.GetDuration("system_probe_config.expected_tags_duration")
	withinAgentStartingPeriod := pkgconfigsetup.StartTime.Add(duration).After(currentTime)

	res := make(map[string]types.EntityID, len(ids))
	// Iterate through the workloadmeta containers, and for the containers whose IDs are in the `ids` map (a.k.a, relevant
	// containers), check if the container started less than `duration` ago. If so, we consider it relevant for explicit
	// tagging and map the container ID to its EntityID.
	_ = d.wmeta.ListContainersWithFilter(func(container *workloadmeta.Container) bool {
		_, ok := ids[container.ID]
		if !ok {
			return false
		}

		// Either the container started less than `duration` ago, or the agent start time is within the `duration` window.
		if withinAgentStartingPeriod || container.State.StartedAt.Add(duration).After(currentTime) {
			res[container.ID] = types.NewEntityID(types.ContainerID, container.ID)
		}
		// No need to actually return the container instance, as we already extracted the relevant information.
		return false
	})
	return res
}

func (d *directSender) addContainerTags(c *model.Connection, containerIDForPID map[int32]string, containersForTagging map[string]types.EntityID, tagsEncoder model.TagEncoder) {
	c.LocalContainerTagsIndex = -1
	c.RemoteServiceTagsIdx = -1
	if c.Laddr.ContainerId == "" {
		return
	}

	containerIDForPID[c.Pid] = c.Laddr.ContainerId
	if entityID, ok := containersForTagging[c.Laddr.ContainerId]; ok {
		if entityTags, err := d.tagger.Tag(entityID, types.HighCardinality); err != nil {
			log.Debugf("error getting tags for container %s: %v", c.Laddr.ContainerId, err)
		} else if len(entityTags) > 0 {
			c.LocalContainerTagsIndex = int32(tagsEncoder.Encode(entityTags))
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
