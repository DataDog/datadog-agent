// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/indexedset"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func formatDNSStatsByDomainByQueryType(builder *model.ConnectionBuilder, stats map[dns.Hostname]map[dns.QueryType]dns.Stats, dnsSet *indexedset.IndexedSet[dns.Hostname], indexToOffset []int32) {
	if indexToOffset == nil {
		return
	}

	for d, bytype := range stats {
		pos := indexToOffset[dnsSet.Add(d)]
		builder.AddDnsStatsByDomainOffsetByQueryType(func(w *model.Connection_DnsStatsByDomainOffsetByQueryTypeEntryBuilder) {
			w.SetKey(pos)
			w.SetValue(func(w *model.DNSStatsByQueryTypeBuilder) {
				for t, stat := range bytype {
					w.AddDnsStatsByQueryType(func(w *model.DNSStatsByQueryType_DnsStatsByQueryTypeEntryBuilder) {
						w.SetKey(int32(t))
						w.SetValue(func(w *model.DNSStatsBuilder) {
							w.SetDnsFailureLatencySum(stat.FailureLatencySum)
							w.SetDnsSuccessLatencySum(stat.SuccessLatencySum)
							w.SetDnsTimeouts(stat.Timeouts)
							for k, v := range stat.CountByRcode {
								w.AddDnsCountByRcode(func(w *model.DNSStats_DnsCountByRcodeEntryBuilder) {
									w.SetKey(k)
									w.SetValue(v)
								})
							}
						})
					})
				}
			})
		})
	}
}

func formatDNSStatsByDomain(builder *model.ConnectionBuilder, stats map[dns.Hostname]map[dns.QueryType]dns.Stats, dnsSet *indexedset.IndexedSet[dns.Hostname], indexToOffset []int32) {
	if indexToOffset == nil {
		return
	}

	ms := &model.DNSStats{DnsCountByRcode: make(map[uint32]uint32)}
	for d, bytype := range stats {
		clear(ms.DnsCountByRcode)
		ms.DnsFailureLatencySum = 0
		ms.DnsSuccessLatencySum = 0
		ms.DnsTimeouts = 0

		builder.AddDnsStatsByDomainOffsetByQueryType(func(w *model.Connection_DnsStatsByDomainOffsetByQueryTypeEntryBuilder) {
			w.SetKey(indexToOffset[dnsSet.Add(d)])
			w.SetValue(func(w *model.DNSStatsByQueryTypeBuilder) {
				for _, stat := range bytype {
					for rcode, count := range stat.CountByRcode {
						ms.DnsCountByRcode[rcode] += count
					}
					ms.DnsFailureLatencySum += stat.FailureLatencySum
					ms.DnsSuccessLatencySum += stat.SuccessLatencySum
					ms.DnsTimeouts += stat.Timeouts
				}
				w.AddDnsStatsByQueryType(func(w *model.DNSStatsByQueryType_DnsStatsByQueryTypeEntryBuilder) {
					w.SetKey(int32(dns.TypeA))
					w.SetValue(func(w *model.DNSStatsBuilder) {
						w.SetDnsFailureLatencySum(ms.DnsFailureLatencySum)
						w.SetDnsSuccessLatencySum(ms.DnsSuccessLatencySum)
						w.SetDnsTimeouts(ms.DnsTimeouts)
						for rcode, count := range ms.DnsCountByRcode {
							w.AddDnsCountByRcode(func(w *model.DNSStats_DnsCountByRcodeEntryBuilder) {
								w.SetKey(rcode)
								w.SetValue(count)
							})
						}
					})
				})
			})
		})
	}
}

func (d *directSender) addDNS(builder *model.ConnectionBuilder, nc network.ConnectionStats, dnsSet *indexedset.IndexedSet[dns.Hostname], indexToOffset []int32) {
	if !d.dnsDomainsEnabled {
		var total, successfulResponses, timeouts uint32
		var successLatencySum, failureLatencySum uint64
		dnsCountByRcode := make(map[uint32]uint32)
		for _, byType := range nc.DNSStats {
			for _, typeStats := range byType {
				successfulResponses += typeStats.CountByRcode[network.DNSResponseCodeNoError]
				timeouts += typeStats.Timeouts
				successLatencySum += typeStats.SuccessLatencySum
				failureLatencySum += typeStats.FailureLatencySum

				for rcode, count := range typeStats.CountByRcode {
					dnsCountByRcode[rcode] += count
					total += count
				}
			}
		}
		builder.SetDnsFailedResponses(total - successfulResponses)
		builder.SetDnsSuccessfulResponses(successfulResponses)
		builder.SetDnsTimeouts(timeouts)
		builder.SetDnsSuccessLatencySum(successLatencySum)
		builder.SetDnsFailureLatencySum(failureLatencySum)
		for k, v := range dnsCountByRcode {
			builder.AddDnsCountByRcode(func(w *model.Connection_DnsCountByRcodeEntryBuilder) {
				w.SetKey(k)
				w.SetValue(v)
			})
		}
	}

	if d.queryTypeEnabled {
		formatDNSStatsByDomainByQueryType(builder, nc.DNSStats, dnsSet, indexToOffset)
	} else {
		formatDNSStatsByDomain(builder, nc.DNSStats, dnsSet, indexToOffset)
	}
}

func getDNSNameForIP(conns *network.Connections, ip util.Address) string {
	if dnsEntry := conns.DNS[ip]; len(dnsEntry) > 0 {
		// We are only using the first entry for now, but in the future, if we find a good solution,
		// we might want to report the other DNS names too if necessary.
		// (need more investigation on how to best achieve that).
		return dnsEntry[0].Get()
	}
	return ""
}
