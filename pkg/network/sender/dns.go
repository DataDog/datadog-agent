// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"maps"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/indexedset"
)

func formatDNSStatsByDomainByQueryType(c *model.Connection, stats map[dns.Hostname]map[dns.QueryType]dns.Stats, dnsSet *indexedset.IndexedSet[dns.Hostname]) {
	c.DnsStatsByDomainByQueryType = make(map[int32]*model.DNSStatsByQueryType, len(stats))
	for d, bytype := range stats {
		pos := dnsSet.Add(d)
		byQueryType := make(map[int32]*model.DNSStats)
		for t, stat := range bytype {
			byQueryType[int32(t)] = &model.DNSStats{
				DnsFailureLatencySum: stat.FailureLatencySum,
				DnsSuccessLatencySum: stat.SuccessLatencySum,
				DnsTimeouts:          stat.Timeouts,
				DnsCountByRcode:      maps.Clone(stat.CountByRcode),
			}
		}
		c.DnsStatsByDomainByQueryType[pos] = &model.DNSStatsByQueryType{
			DnsStatsByQueryType: byQueryType,
		}
	}
}

func formatDNSStatsByDomain(c *model.Connection, stats map[dns.Hostname]map[dns.QueryType]dns.Stats, dnsSet *indexedset.IndexedSet[dns.Hostname]) {
	c.DnsStatsByDomain = make(map[int32]*model.DNSStats, len(stats))
	for d, bytype := range stats {
		pos := dnsSet.Add(d)
		for _, stat := range bytype {
			if ms, ok := c.DnsStatsByDomain[pos]; ok {
				for rcode, count := range stat.CountByRcode {
					ms.DnsCountByRcode[rcode] += count
				}
				ms.DnsFailureLatencySum += stat.FailureLatencySum
				ms.DnsSuccessLatencySum += stat.SuccessLatencySum
				ms.DnsTimeouts += stat.Timeouts
			} else {
				c.DnsStatsByDomain[pos] = &model.DNSStats{
					DnsCountByRcode:      stat.CountByRcode,
					DnsFailureLatencySum: stat.FailureLatencySum,
					DnsSuccessLatencySum: stat.SuccessLatencySum,
					DnsTimeouts:          stat.Timeouts,
				}
			}
		}
	}
}

func remapDNSStatsByOffset(c *model.Connection, indexToOffset []int32) {
	c.DnsStatsByDomainOffsetByQueryType = make(map[int32]*model.DNSStatsByQueryType, len(c.DnsStatsByDomain)+len(c.DnsStatsByDomainByQueryType))

	// first, walk the stats by domain.  Put them in by query type 'A`
	for key, val := range c.DnsStatsByDomain {
		off := indexToOffset[key]
		if _, ok := c.DnsStatsByDomainOffsetByQueryType[off]; !ok {
			c.DnsStatsByDomainOffsetByQueryType[off] = &model.DNSStatsByQueryType{}
			c.DnsStatsByDomainOffsetByQueryType[off].DnsStatsByQueryType = make(map[int32]*model.DNSStats)
		}
		c.DnsStatsByDomainOffsetByQueryType[off].DnsStatsByQueryType[int32(dns.TypeA)] = val
	}
	for key, val := range c.DnsStatsByDomainByQueryType {
		off := indexToOffset[key]
		c.DnsStatsByDomainOffsetByQueryType[off] = val
	}
	c.DnsStatsByDomain = nil
	c.DnsStatsByDomainByQueryType = nil
}

func (d *directSender) addDNS(nc network.ConnectionStats, c *model.Connection, dnsSet *indexedset.IndexedSet[dns.Hostname]) {
	if !d.dnsDomainsEnabled {
		var total uint32
		for _, byType := range nc.DNSStats {
			for _, typeStats := range byType {
				c.DnsSuccessfulResponses += typeStats.CountByRcode[network.DNSResponseCodeNoError]
				c.DnsTimeouts += typeStats.Timeouts
				c.DnsSuccessLatencySum += typeStats.SuccessLatencySum
				c.DnsFailureLatencySum += typeStats.FailureLatencySum

				for rcode, count := range typeStats.CountByRcode {
					c.DnsCountByRcode[rcode] += count
					total += count
				}
			}
		}
		c.DnsFailedResponses = total - c.DnsSuccessfulResponses
	}

	if d.queryTypeEnabled {
		formatDNSStatsByDomainByQueryType(c, nc.DNSStats, dnsSet)
	} else {
		formatDNSStatsByDomain(c, nc.DNSStats, dnsSet)
	}
}
