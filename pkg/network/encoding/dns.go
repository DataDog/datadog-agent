package encoding

import (
	"sync"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"go4.org/intern"
)

var dnsPool = sync.Pool{
	New: func() interface{} {
		return new(model.DNSEntry)
	},
}

type dnsFormatter struct {
	conns     *network.Connections
	ipc       ipCache
	domainSet map[string]int
	seen      map[dns.Key]struct{}

	// Configuration flags
	queryTypeEnabled  bool
	dnsDomainsEnabled bool
}

func newDNSFormatter(conns *network.Connections, ipc ipCache) *dnsFormatter {
	return &dnsFormatter{
		conns:             conns,
		ipc:               ipc,
		domainSet:         make(map[string]int),
		seen:              make(map[dns.Key]struct{}),
		queryTypeEnabled:  config.Datadog.GetBool("network_config.enable_dns_by_querytype"),
		dnsDomainsEnabled: config.Datadog.GetBool("system_probe_config.collect_dns_domains"),
	}
}

func (f *dnsFormatter) FormatConnectionDNS(nc network.ConnectionStats, mc *model.Connection) {
	key, ok := network.DNSKey(&nc)
	if !ok {
		return
	}

	// Avoid overcounting stats in the context of PID collisions
	if _, seen := f.seen[key]; seen {
		return
	}

	// Retrieve DNS information for this particular connection
	stats, ok := f.conns.DNSStats[key]
	if !ok {
		return
	}
	f.seen[key] = struct{}{}

	if !f.dnsDomainsEnabled {
		var total uint32
		mc.DnsCountByRcode = make(map[uint32]uint32)
		for _, byType := range stats {
			for _, typeStats := range byType {
				mc.DnsSuccessfulResponses += typeStats.CountByRcode[network.DNSResponseCodeNoError]
				mc.DnsTimeouts += typeStats.Timeouts
				mc.DnsSuccessLatencySum += typeStats.SuccessLatencySum
				mc.DnsFailureLatencySum += typeStats.FailureLatencySum

				for rcode, count := range typeStats.CountByRcode {
					mc.DnsCountByRcode[rcode] += count
					total += count
				}
			}
		}
		mc.DnsFailedResponses = total - mc.DnsSuccessfulResponses
	}

	if f.queryTypeEnabled {
		mc.DnsStatsByDomain = nil
		mc.DnsStatsByDomainByQueryType = formatDNSStatsByDomainByQueryType(stats, f.domainSet)
	} else {
		// downconvert to simply by domain
		mc.DnsStatsByDomain = formatDNSStatsByDomain(stats, f.domainSet)
		mc.DnsStatsByDomainByQueryType = nil
	}
	mc.DnsStatsByDomainOffsetByQueryType = nil

}

func (f *dnsFormatter) DNS() map[string]*model.DNSEntry {
	if f.conns.DNS == nil {
		return nil
	}

	ipToNames := make(map[string]*model.DNSEntry, len(f.conns.DNS))
	for addr, names := range f.conns.DNS {
		entry := dnsPool.Get().(*model.DNSEntry)
		entry.Names = names
		ipToNames[f.ipc.Get(addr)] = entry
	}

	return ipToNames
}

func (f *dnsFormatter) Domains() []string {
	domains := make([]string, len(f.domainSet))
	for k, v := range f.domainSet {
		domains[v] = k
	}
	return domains
}

func formatDNSStatsByDomainByQueryType(stats map[*intern.Value]map[dns.QueryType]dns.Stats, domainSet map[string]int) map[int32]*model.DNSStatsByQueryType {
	m := make(map[int32]*model.DNSStatsByQueryType)
	for d, bytype := range stats {

		byqtype := &model.DNSStatsByQueryType{}
		byqtype.DnsStatsByQueryType = make(map[int32]*model.DNSStats)
		for t, stat := range bytype {
			var ms model.DNSStats
			ms.DnsCountByRcode = stat.CountByRcode
			ms.DnsFailureLatencySum = stat.FailureLatencySum
			ms.DnsSuccessLatencySum = stat.SuccessLatencySum
			ms.DnsTimeouts = stat.Timeouts
			byqtype.DnsStatsByQueryType[int32(t)] = &ms
		}
		pos, ok := domainSet[d.Get().(string)]
		if !ok {
			pos = len(domainSet)
			domainSet[d.Get().(string)] = pos
		}
		m[int32(pos)] = byqtype
	}
	return m
}

func formatDNSStatsByDomain(stats map[*intern.Value]map[dns.QueryType]dns.Stats, domainSet map[string]int) map[int32]*model.DNSStats {
	m := make(map[int32]*model.DNSStats)
	for d, bytype := range stats {
		pos, ok := domainSet[d.Get().(string)]
		if !ok {
			pos = len(domainSet)
			domainSet[d.Get().(string)] = pos
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
	return m
}
