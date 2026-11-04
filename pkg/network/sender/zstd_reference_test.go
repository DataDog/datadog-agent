// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

// Reference copy of the streaming-writer batch encoder, used to prove that
// bulk compression with a reused context leaves the decompressed payload
// unchanged.

import (
	"bytes"
	"io"
	"iter"
	"slices"
	"strconv"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/encoding/marshal"
	"github.com/DataDog/datadog-agent/pkg/network/indexedset"
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
	for _, c := range conns.Conns {
		// USM supports TCP only; skip UDP connections.
		if c.IntraHost && c.Pid > 0 && c.SPort > 0 && c.Type == network.TCP {
			key := listenKey{IP: c.Source.Addr, Port: int32(c.SPort)}
			if _, exists := listeners[key]; !exists {
				listeners[key] = int32(c.Pid)
			}
		}
	}
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
					d.addContainerTags(builder, nc.ContainerID.Source, tagsEncoder)
					d.addRemoteServiceTags(builder, nc, remoteServiceResolver, tagsEncoder)
					d.addTags(builder, nc, tagsSet, usmEncoders, connectionsTagsEncoder)
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
