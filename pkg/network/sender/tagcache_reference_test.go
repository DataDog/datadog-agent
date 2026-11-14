// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

// Reference copy of the uncached batch encoder, used to prove that the tag
// cache leaves every payload byte unchanged.

import (
	"bytes"
	"io"
	"iter"
	"maps"
	"slices"
	"strconv"

	model "github.com/DataDog/agent-payload/v5/process"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/encoding/marshal"
	"github.com/DataDog/datadog-agent/pkg/network/indexedset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/zstd"
)

func (d *directSender) batchesReference(conns *network.Connections, groupID int32) iter.Seq[[]byte] {
	messageIndex := 0
	numBatches := d.batchCount(conns)
	dnsEncoder := model.NewV2DNSEncoder()
	tagsSet := indexedset.New[string](0)

	usmEncoders := marshal.InitializeUSMEncoders(conns)
	d.resolver.resolveDestinationContainerIDs(conns)

	iisTags, procCacheTags, listeners := fetchServiceData(d.tracer)
	if listeners == nil {
		listeners = make(map[listenKey]int32)
	}
	var getIISTags func(remotePort, localPort int32) []string
	if iisTags != nil {
		getIISTags = func(remotePort, localPort int32) []string {
			iisKey := strconv.FormatInt(int64(remotePort), 10) + "-" + strconv.FormatInt(int64(localPort), 10)
			if tags, ok := iisTags[iisKey]; ok {
				return tags
			}
			return nil
		}
	}

	// Build remote service resolver for intra-host connection enrichment.
	addConnectionListeners(listeners, conns.Conns)
	remoteServiceResolver := &serviceResolver{
		GetServiceContext: func(pid int32) []string {
			if dsc := directSenderConsumerInstance.Load(); dsc != nil {
				return dsc.extractor.GetServiceContext(pid)
			}
			return nil
		},
		GetProcessTags: func(pid int32) []string {
			return getProcessTags(pid, procCacheTags, func(_ int32) ([]string, error) {
				if pid <= 0 {
					return nil, nil
				}
				processEntityID := types.NewEntityID(types.Process, strconv.Itoa(int(pid)))
				return d.tagger.Tag(processEntityID, types.HighCardinality)
			})
		},
		GetIISTags: getIISTags,
		Listeners:  listeners,
	}

	// Sort connections by remote IP/PID for more efficient resolution
	slices.SortFunc(conns.Conns, func(a, b network.ConnectionStats) int {
		if a.Dest.Addr != b.Dest.Addr {
			return a.Dest.Addr.Compare(b.Dest.Addr)
		}
		return int(a.Pid) - int(b.Pid)
	})

	builder := model.NewCollectorConnectionsBuilder(io.Discard)

	return func(yield func([]byte) bool) {
		defer func() {
			for _, e := range usmEncoders {
				e.Close()
			}
		}()
		defer d.resolver.removeDeadTagContainers()

		for connsChunk := range slices.Chunk(conns.Conns, d.maxConnsPerMessage) {
			// TODO is there some way to get a larger lower bound on the size of a payload
			// ex: (minConnSize * len(connsChunk)) + len(d.staticEncodedHeader)
			// compression makes this calculation difficult
			dstBuf := bytes.NewBuffer(make([]byte, 0, len(d.staticEncodedHeader)))
			_, err := dstBuf.Write(d.staticEncodedHeader)
			if err != nil {
				d.log.Errorf("Unable to encode message header: %s", err)
				continue
			}
			zw, err := zstd.NewWriter(dstBuf)
			if err != nil {
				d.log.Errorf("Unable to create zstd writer: %s", err)
				continue
			}

			builder.Reset(zw)
			d.encodeConfiguration(builder)
			builder.SetGroupId(groupID)
			builder.SetGroupSize(numBatches)

			if messageIndex == 0 {
				d.encodeTelemetry(conns, builder)
			}

			d.encodeContainerForPID(connsChunk, builder)
			dnsSet, indexToOffset := d.encodeDNS(connsChunk, conns, dnsEncoder, builder)
			resolvConfSet := indexedset.New[network.ResolvConf](0)
			routeSet := indexedset.New[network.Via](0)
			connectionsTagsEncoder := model.NewV3TagEncoder()
			tagsEncoder := model.NewV3TagEncoder()
			// Adding a dummy tag to ensure the indices we get are always >= 0.
			_ = tagsEncoder.Encode([]string{"-"})

			for _, nc := range connsChunk {
				builder.AddConnections(func(builder *model.ConnectionBuilder) {
					d.encodeConnection(builder, nc, conns, routeSet, resolvConfSet)
					d.addContainerTagsReference(builder, nc.ContainerID.Source, tagsEncoder)
					d.addRemoteServiceTags(builder, nc, remoteServiceResolver, tagsEncoder)
					d.addTagsReference(builder, nc, tagsSet, usmEncoders, connectionsTagsEncoder)
					d.addDNS(builder, nc, dnsSet, indexToOffset)
				})
			}

			// this must come before we access tagsEncoder.Buffer()
			builder.SetHostTagsIndex(int32(tagsEncoder.Encode(d.hostTagProvider.GetHostTags())))
			builder.SetEncodedTags(func(b *bytes.Buffer) {
				b.Write(tagsEncoder.Buffer())
			})
			builder.SetEncodedConnectionsTags(func(b *bytes.Buffer) {
				b.Write(connectionsTagsEncoder.Buffer())
			})
			d.encodeRoutes(routeSet, builder)
			for _, v := range resolvConfSet.UniqueKeys() {
				builder.AddResolvConfs(v.Get())
			}

			if err := zw.Close(); err != nil {
				d.log.Errorf("Unable to close compression writer: %s", err)
				continue
			}

			if !yield(dstBuf.Bytes()) {
				return
			}
			messageIndex++
		}
	}
}

func (d *directSender) addContainerTagsReference(builder *model.ConnectionBuilder, sourceContainerID *intern.Value, tagsEncoder model.TagEncoder) {
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

func (d *directSender) addTagsReference(builder *model.ConnectionBuilder, nc network.ConnectionStats, tagsSet *indexedset.IndexedSet[string], usmEncoders []marshal.USMEncoder, connectionsTagsEncoder model.TagEncoder) {
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

	tagsStr := tagsSet.Subset(tagIndexes)
	if nc.Pid > 0 {
		reattributed := nc.NetNSOriginalContainerID != nil
		var serviceTags []string
		if dsc := directSenderConsumerInstance.Load(); dsc != nil {
			if !reattributed {
				serviceTags = dsc.extractor.GetServiceContext(int32(nc.Pid))
			}
			if processName := dsc.processNameExtractor.GetProcessName(int32(nc.Pid)); processName != "" {
				tagsStr = append(tagsStr, "process_name:"+processName)
			}
		}
		tagsStr = append(tagsStr, serviceTags...)
		if !reattributed {
			processEntityID := types.NewEntityID(types.Process, strconv.Itoa(int(nc.Pid)))
			if processTags, err := d.tagger.Tag(processEntityID, types.HighCardinality); err != nil {
				log.Debugf("error getting tags for process %v: %v", nc.Pid, err)
			} else {
				tagsStr = append(tagsStr, processTags...)
			}
		}
	}
	if len(tagsStr) > 0 {
		builder.SetTagsIdx(int32(connectionsTagsEncoder.Encode(tagsStr)))
	} else {
		builder.SetTagsIdx(-1)
	}
}
