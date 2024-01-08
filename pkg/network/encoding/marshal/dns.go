// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(NET) Fix revive linter
package marshal

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
)

type dnsFormatter struct {
	conns     *network.Connections
	ipc       ipCache
	domainSet map[string]int

	// Configuration flags
	queryTypeEnabled  bool
	dnsDomainsEnabled bool
}

func newDNSFormatter(conns *network.Connections, ipc ipCache) *dnsFormatter {
	return &dnsFormatter{
		conns:             conns,
		ipc:               ipc,
		domainSet:         make(map[string]int),
		queryTypeEnabled:  config.SystemProbe.GetBool("network_config.enable_dns_by_querytype"),
		dnsDomainsEnabled: config.SystemProbe.GetBool("system_probe_config.collect_dns_domains"),
	}
}

func (f *dnsFormatter) FormatConnectionDNS(nc network.ConnectionStats, builder *model.ConnectionBuilder) {
	var (
		dnsCountByRcode        map[uint32]uint32
		dnsSuccessfulResponses uint32
		dnsTimeouts            uint32
		dnsSuccessLatencySum   uint64
		dnsFailureLatencySum   uint64
		dnsFailedResponses     uint32
	)

	if !f.dnsDomainsEnabled {
		var total uint32
		dnsCountByRcode = make(map[uint32]uint32)
		for _, byType := range nc.DNSStats {
			for _, typeStats := range byType {
				dnsSuccessfulResponses += typeStats.CountByRcode[network.DNSResponseCodeNoError]
				dnsTimeouts += typeStats.Timeouts
				dnsSuccessLatencySum += typeStats.SuccessLatencySum
				dnsFailureLatencySum += typeStats.FailureLatencySum

				for rcode, count := range typeStats.CountByRcode {
					dnsCountByRcode[rcode] += count
					total += count
				}
			}
		}
		dnsFailedResponses = total - dnsSuccessfulResponses
	}

	for k, v := range dnsCountByRcode {
		builder.AddDnsCountByRcode(func(w *model.Connection_DnsCountByRcodeEntryBuilder) {
			w.SetKey(k)
			w.SetValue(v)
		})
	}
	builder.SetDnsSuccessfulResponses(dnsSuccessfulResponses)
	builder.SetDnsTimeouts(dnsTimeouts)
	builder.SetDnsSuccessLatencySum(dnsSuccessLatencySum)
	builder.SetDnsFailureLatencySum(dnsFailureLatencySum)
	builder.SetDnsFailedResponses(dnsFailedResponses)

	if f.queryTypeEnabled {
		formatDNSStatsByDomainByQueryType(builder, nc.DNSStats, f.domainSet)
	} else {
		//// downconvert to simply by domain
		formatDNSStatsByDomain(builder, nc.DNSStats, f.domainSet)
	}
}

// FormatDNS writes the DNS field in the Connections object
func (f *dnsFormatter) FormatDNS(builder *model.ConnectionsBuilder) {
	if f.conns.DNS == nil {
		return
	}

	for addr, names := range f.conns.DNS {
		builder.AddDns(func(w *model.Connections_DnsEntryBuilder) {
			w.SetKey(f.ipc.Get(addr))
			w.SetValue(func(w *model.DNSEntryBuilder) {
				for _, name := range names {
					w.AddNames(dns.ToString(name))
				}
			})
		})
	}
}

func (f *dnsFormatter) Domains() []string {
	domains := make([]string, len(f.domainSet))
	for k, v := range f.domainSet {
		domains[v] = k
	}
	return domains
}

func formatDNSStatsByDomainByQueryType(builder *model.ConnectionBuilder, stats map[dns.Hostname]map[dns.QueryType]dns.Stats, domainSet map[string]int) {
	for d, bytype := range stats {
		pos, ok := domainSet[dns.ToString(d)]
		if !ok {
			pos = len(domainSet)
			domainSet[dns.ToString(d)] = pos
		}

		builder.AddDnsStatsByDomainByQueryType(func(w *model.Connection_DnsStatsByDomainByQueryTypeEntryBuilder) {
			w.SetKey(int32(pos))
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

func formatDNSStatsByDomain(builder *model.ConnectionBuilder, stats map[dns.Hostname]map[dns.QueryType]dns.Stats, domainSet map[string]int) {
	m := make(map[int32]*model.DNSStats)
	for d, bytype := range stats {
		pos, ok := domainSet[dns.ToString(d)]
		if !ok {
			pos = len(domainSet)
			domainSet[dns.ToString(d)] = pos
		}

		for _, stat := range bytype {

			if ms, ok := m[int32(pos)]; ok {
				for rcode, count := range stat.CountByRcode {
					ms.DnsCountByRcode[rcode] += count
				}
				ms.DnsFailureLatencySum += stat.FailureLatencySum
				ms.DnsSuccessLatencySum += stat.SuccessLatencySum
				ms.DnsTimeouts += stat.Timeouts

			} else {
				var ms model.DNSStats
				ms.DnsCountByRcode = stat.CountByRcode
				ms.DnsFailureLatencySum = stat.FailureLatencySum
				ms.DnsSuccessLatencySum = stat.SuccessLatencySum
				ms.DnsTimeouts = stat.Timeouts

				m[int32(pos)] = &ms
			}
		}
	}

	for k, v := range m {
		builder.AddDnsStatsByDomain(func(w *model.Connection_DnsStatsByDomainEntryBuilder) {
			w.SetKey(k)
			w.SetValue(func(w *model.DNSStatsBuilder) {
				w.SetDnsTimeouts(v.DnsTimeouts)
				w.SetDnsFailureLatencySum(v.DnsFailureLatencySum)
				w.SetDnsSuccessLatencySum(v.DnsSuccessLatencySum)
				for rcode, count := range v.DnsCountByRcode {
					w.AddDnsCountByRcode(func(w *model.DNSStats_DnsCountByRcodeEntryBuilder) {
						w.SetKey(rcode)
						w.SetValue(count)
					})
				}
			})
		})
	}
}
