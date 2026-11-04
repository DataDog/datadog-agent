// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux || (windows && npm)

package sender

import (
	"encoding/binary"
	"maps"
	"slices"
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

// chunkTagCache memoizes tag-encoder indices for the connections of one chunk.
// The indices belong to that chunk's encoders, so it must be reset per chunk.
type chunkTagCache struct {
	container map[*intern.Value]int32
	// pid holds connections without formatTags tags; conn holds the rest,
	// keyed by connKey.
	pid    map[pidTagKey]int32
	conn   map[string]int32
	keyBuf []byte
}

// pidTagKey identifies the tags a PID contributes: a reattributed connection's PID belongs to the proxy, so it
// contributes fewer tags than the same PID's own connections.
type pidTagKey struct {
	pid          uint32
	reattributed bool
}

func newChunkTagCache() *chunkTagCache {
	return &chunkTagCache{
		container: make(map[*intern.Value]int32),
		pid:       make(map[pidTagKey]int32),
		conn:      make(map[string]int32),
	}
}

func (c *chunkTagCache) reset() {
	clear(c.container)
	clear(c.pid)
	clear(c.conn)
}

// connKey encodes every input of the connection tag set: the PID, whether the
// connection is reattributed, and the sorted formatTags indices. It sorts
// tagIndexes in place.
func (c *chunkTagCache) connKey(pid uint32, reattributed bool, tagIndexes []int32) []byte {
	slices.Sort(tagIndexes)
	c.keyBuf = binary.LittleEndian.AppendUint32(c.keyBuf[:0], pid)
	if reattributed {
		c.keyBuf = append(c.keyBuf, 1)
	} else {
		c.keyBuf = append(c.keyBuf, 0)
	}
	for _, idx := range tagIndexes {
		c.keyBuf = binary.LittleEndian.AppendUint32(c.keyBuf, uint32(idx))
	}
	return c.keyBuf
}

func (d *directSender) addContainerTags(builder *model.ConnectionBuilder, sourceContainerID *intern.Value, tagsEncoder model.TagEncoder, cache *chunkTagCache) {
	if sourceContainerID == nil {
		builder.SetLocalContainerTagsIndex(-1)
		return
	}
	idx, ok := cache.container[sourceContainerID]
	if !ok {
		idx = d.containerTagsIndex(sourceContainerID, tagsEncoder)
		cache.container[sourceContainerID] = idx
	}
	builder.SetLocalContainerTagsIndex(idx)
}

func (d *directSender) containerTagsIndex(sourceContainerID *intern.Value, tagsEncoder model.TagEncoder) int32 {
	if d.resolver.shouldTagContainer(sourceContainerID) {
		if cid := getInternedString(sourceContainerID); cid != "" {
			if entityTags, err := d.tagger.Tag(types.NewEntityID(types.ContainerID, cid), types.HighCardinality); err != nil {
				if log.ShouldLog(log.DebugLvl) {
					log.Debugf("error getting tags for container %s: %v", cid, err)
				}
			} else if len(entityTags) > 0 {
				return int32(tagsEncoder.Encode(entityTags))
			}
		}
	}
	return -1
}

func (d *directSender) addRemoteServiceTags(builder *model.ConnectionBuilder, nc network.ConnectionStats, resolver *serviceResolver, tagsEncoder model.TagEncoder) {
	srcContainerID := getInternedString(nc.ContainerID.Source)
	// USM supports TCP only; skip UDP connections.
	if nc.IntraHost && srcContainerID == "" && nc.Type == network.TCP {
		if remoteTags := resolver.Resolve(int32(nc.Pid), nc.Dest.Addr, int32(nc.DPort), int32(nc.SPort)); len(remoteTags) > 0 {
			builder.SetRemoteServiceTagsIdx(int32(tagsEncoder.Encode(remoteTags)))
			return
		}
	}
	builder.SetRemoteServiceTagsIdx(-1)
}

func (d *directSender) addTags(builder *model.ConnectionBuilder, nc network.ConnectionStats, tagsSet *indexedset.IndexedSet[string], usmEncoders []marshal.USMEncoder, connectionsTagsEncoder model.TagEncoder, cache *chunkTagCache) {
	var staticTags uint64
	dynamicTags := nc.TLSTags.GetDynamicTags()
	for _, encoder := range usmEncoders {
		encoderStaticTags, encoderDynamicTags := encoder.EncodeConnection(nc, builder)
		staticTags |= encoderStaticTags
		if dynamicTags == nil {
			dynamicTags = make(map[string]struct{})
		}
		maps.Copy(dynamicTags, encoderDynamicTags)
	}

	nc.StaticTags |= staticTags
	tagIndexes, tagChecksum := formatTags(nc, tagsSet, dynamicTags)
	builder.SetTagsChecksum(tagChecksum)

	reattributed := nc.NetNSOriginalContainerID != nil
	if len(tagIndexes) == 0 {
		pk := pidTagKey{pid: nc.Pid, reattributed: reattributed}
		idx, ok := cache.pid[pk]
		if !ok {
			idx = d.connectionTagsIndex(nc.Pid, reattributed, nil, connectionsTagsEncoder)
			cache.pid[pk] = idx
		}
		builder.SetTagsIdx(idx)
		return
	}
	key := cache.connKey(nc.Pid, reattributed, tagIndexes)
	idx, ok := cache.conn[string(key)]
	if !ok {
		idx = d.connectionTagsIndex(nc.Pid, reattributed, tagsSet.Subset(tagIndexes), connectionsTagsEncoder)
		cache.conn[string(key)] = idx
	}
	builder.SetTagsIdx(idx)
}

func (d *directSender) connectionTagsIndex(pid uint32, reattributed bool, tagsStr []string, connectionsTagsEncoder model.TagEncoder) int32 {
	if pid > 0 {
		// a socket reattributed to its network namespace's container is still owned by the proxy
		// process, so the PID's service context and process tags describe the proxy, not the pod
		var serviceTags []string
		if dsc := directSenderConsumerInstance.Load(); dsc != nil {
			if !reattributed {
				serviceTags = dsc.extractor.GetServiceContext(int32(pid))
			}
			// process_name is kept for reattributed connections: it names the socket's owning process
			// (for example ztunnel), not a service, and lets users find proxied traffic
			if processName := dsc.processNameExtractor.GetProcessName(int32(pid)); processName != "" {
				tagsStr = append(tagsStr, "process_name:"+processName)
			}
		}
		tagsStr = append(tagsStr, serviceTags...)
		if !reattributed {
			processEntityID := types.NewEntityID(types.Process, strconv.Itoa(int(pid)))
			if processTags, err := d.tagger.Tag(processEntityID, types.HighCardinality); err != nil {
				log.Debugf("error getting tags for process %v: %v", pid, err)
			} else {
				tagsStr = append(tagsStr, processTags...)
			}
		}
	}
	if len(tagsStr) > 0 {
		return int32(connectionsTagsEncoder.Encode(tagsStr))
	}
	return -1
}
