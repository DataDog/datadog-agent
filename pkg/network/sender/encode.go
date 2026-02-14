// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"bytes"
	"unique"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/encoding/marshal"
	"github.com/DataDog/datadog-agent/pkg/network/indexedset"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	ddslices "github.com/DataDog/datadog-agent/pkg/util/slices"
)

func (d *directSender) encodeConnection(
	builder *model.ConnectionBuilder,
	nc network.ConnectionStats,
	conns *network.Connections,
	routeSet *indexedset.IndexedSet[network.Via],
	resolvConfSet *indexedset.IndexedSet[network.ResolvConf],
) {
	builder.SetPid(int32(nc.Pid))
	builder.SetLaddr(func(w *model.AddrBuilder) {
		w.SetIp(unique.Make(nc.Source.Addr.String()).Value())
		w.SetPort(int32(nc.SPort))
		w.SetContainerId(getInternedString(nc.ContainerID.Source))
	})
	builder.SetRaddr(func(w *model.AddrBuilder) {
		w.SetIp(unique.Make(nc.Dest.Addr.String()).Value())
		w.SetPort(int32(nc.DPort))
		w.SetContainerId(getInternedString(nc.ContainerID.Dest))
	})
	builder.SetFamily(uint64(formatFamily(nc.Family)))
	builder.SetType(uint64(formatType(nc.Type)))
	builder.SetIsLocalPortEphemeral(uint64(formatEphemeralType(nc.SPortIsEphemeral)))
	builder.SetLastBytesSent(nc.Last.SentBytes)
	builder.SetLastBytesReceived(nc.Last.RecvBytes)
	builder.SetLastPacketsSent(nc.Last.SentPackets)
	builder.SetLastRetransmits(nc.Last.Retransmits)
	builder.SetLastPacketsReceived(nc.Last.RecvPackets)
	builder.SetDirection(uint64(formatDirection(nc.Direction)))
	builder.SetNetNS(nc.NetNS)
	if nc.IPTranslation != nil {
		builder.SetIpTranslation(func(w *model.IPTranslationBuilder) {
			w.SetReplSrcIP(unique.Make(nc.IPTranslation.ReplSrcIP.Addr.String()).Value())
			w.SetReplDstIP(unique.Make(nc.IPTranslation.ReplDstIP.Addr.String()).Value())
			w.SetReplSrcPort(int32(nc.IPTranslation.ReplSrcPort))
			w.SetReplDstPort(int32(nc.IPTranslation.ReplDstPort))
		})
	}
	builder.SetRtt(nc.RTT)
	builder.SetRttVar(nc.RTTVar)
	builder.SetIntraHost(nc.IntraHost)
	builder.SetLastTcpEstablished(uint32(nc.Last.TCPEstablished))
	builder.SetLastTcpClosed(uint32(nc.Last.TCPClosed))
	builder.SetProtocol(func(w *model.ProtocolStackBuilder) {
		for p := range marshal.FormatProtocolStack(nc.ProtocolStack, nc.StaticTags) {
			w.AddStack(uint64(p))
		}
	})
	builder.SetRouteIdx(formatRouteIndex(nc.Via, routeSet))
	for k, v := range nc.TCPFailures {
		builder.AddTcpFailuresByErrCode(func(w *model.Connection_TcpFailuresByErrCodeEntryBuilder) {
			w.SetKey(uint32(k))
			w.SetValue(v)
		})
	}
	builder.SetSystemProbeConn(nc.Pid == d.sysProbePID)
	if resolvConf, ok := conns.ResolvConfs[nc.ContainerID.Source]; ok {
		builder.SetResolvConfIdx(resolvConfSet.Add(resolvConf))
	} else {
		builder.SetResolvConfIdx(-1)
	}
}

func (d *directSender) encodeDNS(connsChunk []network.ConnectionStats, conns *network.Connections, dnsEncoder model.DNSEncoderV2, builder *model.CollectorConnectionsBuilder) (*indexedset.IndexedSet[dns.Hostname], []int32) {
	dnsCount := 0
	dnsHostnameCount := 0
	for _, nc := range connsChunk {
		if dnsHostnames, ok := conns.DNS[nc.Dest]; ok {
			dnsCount++
			dnsHostnameCount += len(dnsHostnames)
		}
	}
	// TODO more efficient to use *intern.Value for map key
	// TODO more efficient to just use []int32 name offsets for value
	// DNS values for this batch only. Subset of conns.DNS
	dnsForBatch := make(map[string]*model.DNSDatabaseEntry, dnsCount)
	// completely populate dns set ahead of time
	dnsSet := indexedset.New[dns.Hostname](dnsHostnameCount)
	for _, nc := range connsChunk {
		destIP := unique.Make(nc.Dest.Addr.String()).Value()
		// create unique DNSDatabaseEntry values
		if dnsHostnames, ok := conns.DNS[nc.Dest]; ok {
			if _, present := dnsForBatch[destIP]; !present {
				dnsForBatch[destIP] = &model.DNSDatabaseEntry{
					NameOffsets: dnsSet.AddSlice(dnsHostnames),
				}
			}
		}
		for d := range nc.DNSStats {
			dnsSet.Add(d)
		}
	}
	// convert collected DNS information for this batch to an optimized version for transmission
	uniqDNSStringList := ddslices.Map(dnsSet.UniqueKeys(), func(h dns.Hostname) string { return dns.ToString(h) })
	encodedNameDb, indexToOffset, err := dnsEncoder.EncodeDomainDatabase(uniqDNSStringList)
	if err != nil {
		// since we were unable to properly encode the indexToOffset map, the
		// rest of the maps will now be unreadable by the back-end.  Just clear them
		indexToOffset = nil
	} else {
		builder.SetEncodedDomainDatabase(func(b *bytes.Buffer) {
			b.Write(encodedNameDb)
		})

		// Now we have all available information.  EncodeMapped with take the string indices
		// that are used, and encode (using the indexToOffset array) the offset into the buffer
		// this way individual strings can be directly accessed on decode.
		mappedDNSLookups, err := dnsEncoder.EncodeMapped(dnsForBatch, indexToOffset)
		if err == nil && mappedDNSLookups != nil {
			builder.SetEncodedDnsLookups(func(b *bytes.Buffer) {
				b.Write(mappedDNSLookups)
			})
		}
	}
	return dnsSet, indexToOffset
}

func (d *directSender) encodeContainerForPID(connsChunk []network.ConnectionStats, builder *model.CollectorConnectionsBuilder) {
	cidCount := 0
	for _, conn := range connsChunk {
		if conn.ContainerID.Source != nil {
			cidCount++
		}
	}
	// cidCount will be an upper bound, only hit if:
	// every connection with a ContainerID has a unique PID
	writtenPids := make(map[uint32]struct{}, cidCount)
	for _, conn := range connsChunk {
		if conn.ContainerID.Source != nil {
			if _, ok := writtenPids[conn.Pid]; !ok {
				writtenPids[conn.Pid] = struct{}{}
				builder.AddContainerForPid(func(w *model.CollectorConnections_ContainerForPidEntryBuilder) {
					w.SetKey(int32(conn.Pid))
					w.SetValue(getInternedString(conn.ContainerID.Source))
				})
			}
		}
	}
}

func (d *directSender) encodeTelemetry(conns *network.Connections, builder *model.CollectorConnectionsBuilder) {
	// only add the telemetry to the first message to prevent double counting
	for k, v := range conns.ConnTelemetry {
		builder.AddConnTelemetryMap(func(w *model.CollectorConnections_ConnTelemetryMapEntryBuilder) {
			w.SetKey(string(k))
			w.SetValue(v)
		})
	}
	for k, v := range conns.CompilationTelemetryByAsset {
		builder.AddCompilationTelemetryByAsset(func(w *model.CollectorConnections_CompilationTelemetryByAssetEntryBuilder) {
			w.SetKey(k)
			w.SetValue(func(w *model.RuntimeCompilationTelemetryBuilder) {
				w.SetRuntimeCompilationEnabled(v.RuntimeCompilationEnabled)
				w.SetRuntimeCompilationResult(uint64(v.RuntimeCompilationResult))
				w.SetRuntimeCompilationDuration(v.RuntimeCompilationDuration)
			})
		})
	}
	for k, v := range conns.CORETelemetryByAsset {
		builder.AddCORETelemetryByAsset(func(w *model.CollectorConnections_CORETelemetryByAssetEntryBuilder) {
			w.SetKey(k)
			w.SetValue(uint64(v))
		})
	}
	for _, asset := range conns.PrebuiltAssets {
		builder.AddPrebuiltEBPFAssets(asset)
	}
	builder.SetKernelHeaderFetchResult(uint64(conns.KernelHeaderFetchResult))
}

func (d *directSender) encodeConfiguration(builder *model.CollectorConnectionsBuilder) {
	builder.SetAgentConfiguration(func(w *model.AgentConfigurationBuilder) {
		w.SetCsmEnabled(d.agentCfg.CsmEnabled)
		w.SetCcmEnabled(d.agentCfg.CcmEnabled)
		w.SetDsmEnabled(d.agentCfg.DsmEnabled)
		w.SetNpmEnabled(d.agentCfg.NpmEnabled)
		w.SetUsmEnabled(d.agentCfg.UsmEnabled)
	})
	builder.SetContainerHostType(uint64(d.containerHostType))
	builder.SetHostName(d.hostname)
	builder.SetNetworkId(d.networkID)
	kernelVersion, _ := kernel.Release()
	builder.SetKernelVersion(kernelVersion)
	architecture, _ := kernel.Machine()
	builder.SetArchitecture(architecture)
	// TODO do we want to try some auto-correction for incorrect platforms, like we do with BTF?
	platform, _ := kernel.Platform()
	builder.SetPlatform(platform)
	platformVersion, _ := kernel.PlatformVersion()
	builder.SetPlatformVersion(platformVersion)
}

func (d *directSender) encodeRoutes(routeSet *indexedset.IndexedSet[network.Via], builder *model.CollectorConnectionsBuilder) {
	for _, v := range routeSet.UniqueKeys() {
		builder.AddRoutes(func(w *model.RouteBuilder) {
			if v.Subnet.Alias != "" {
				w.SetSubnet(func(w *model.SubnetBuilder) {
					w.SetAlias(v.Subnet.Alias)
				})
			}
			if v.Interface.HardwareAddr != "" {
				w.SetInterface(func(w *model.InterfaceBuilder) {
					w.SetHardwareAddr(v.Interface.HardwareAddr)
				})
			}
		})
	}
}
